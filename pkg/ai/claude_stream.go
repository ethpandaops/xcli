package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// askStreamState accumulates a turn's output and diagnostics as protocol
// messages arrive.
type askStreamState struct {
	seq              int
	assistantText    strings.Builder
	resultText       string
	streamText       strings.Builder
	assistantErr     string
	resultErr        string
	msgCount         int
	streamEventCount int
	assistantCount   int
	resultCount      int
	streamChars      int
	thinkingChars    int
	assistantChars   int
	resultChars      int
	lastMessageType  string
	lastEventType    string
	resultSubtype    string
	resultIsError    bool
	endedWithEOF     bool
	currentToolName  string
	toolInputBuf     strings.Builder
}

func (st *askStreamState) debugInfo(s *claudeSession, prompt string) map[string]any {
	return map[string]any{
		"session_id":          s.id,
		"prompt_len":          len(prompt),
		"msg_count":           st.msgCount,
		"stream_event_count":  st.streamEventCount,
		"assistant_count":     st.assistantCount,
		"result_count":        st.resultCount,
		"stream_chars":        st.streamChars,
		"thinking_chars":      st.thinkingChars,
		"assistant_chars":     st.assistantChars,
		"result_chars":        st.resultChars,
		"assistant_err":       st.assistantErr,
		"result_err":          st.resultErr,
		"result_subtype":      st.resultSubtype,
		"result_is_error":     st.resultIsError,
		"last_message_type":   st.lastMessageType,
		"last_stream_event":   st.lastEventType,
		"client_timeout_secs": int(s.timeout.Seconds()),
		"ended_with_eof":      st.endedWithEOF,
		"stderr_count":        s.stderrCount(),
		"stderr_tail":         s.stderrTail(20),
	}
}

func (st *askStreamState) handleReceiveError(err error, debugInfo func() map[string]any) (bool, error) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true, err
	}

	if errors.Is(err, io.EOF) {
		st.endedWithEOF = true

		return true, nil
	}

	if strings.Contains(strings.ToLower(err.Error()), "interrupt") {
		return true, err
	}

	return true, &ProviderDebugError{Cause: err, Info: debugInfo()}
}

func (st *askStreamState) handleMessage(
	msg any,
	onChunk func(StreamChunk),
	debugInfo func() map[string]any,
) (string, bool, error) {
	st.msgCount++

	switch m := msg.(type) {
	case *streamEvent:
		st.lastMessageType = "stream_event"
		st.streamEventCount++

		if eventType, ok := m.Event["type"].(string); ok {
			st.lastEventType = eventType
		}

		st.seq += streamEventChunks(m, st.seq, st, onChunk)

		if parts, ok := parseStreamEvent(m); ok {
			if parts.Thinking != "" {
				st.thinkingChars += len(parts.Thinking)
			}

			if parts.Text != "" {
				st.streamText.WriteString(parts.Text)
				st.streamChars += len(parts.Text)
			}
		}
	case *assistantMessage:
		st.lastMessageType = "assistant"
		st.assistantCount++

		if m.Error != nil {
			st.assistantErr = *m.Error
		}

		for _, block := range m.Content {
			if strings.TrimSpace(block.Text) == "" {
				continue
			}

			if st.assistantText.Len() > 0 {
				st.assistantText.WriteString("\n")
			}

			st.assistantText.WriteString(block.Text)
			st.assistantChars += len(block.Text)

			// Some CLI runs only surface thinking in stream events and provide
			// answer text via assistant messages before final result. Emit fallback
			// answer chunks so the UI streams visible answer text during the turn.
			if onChunk != nil && st.streamChars == 0 {
				st.seq++
				onChunk(StreamChunk{
					Kind:      StreamChunkAnswer,
					Text:      block.Text,
					EventType: "assistant_message",
					Seq:       st.seq,
				})
			}
		}
	case *resultMessage:
		st.lastMessageType = "result"
		st.resultCount++
		st.resultSubtype = m.Subtype
		st.resultIsError = m.IsError

		if m.IsError {
			st.resultErr = m.Subtype

			if m.Result != nil && strings.TrimSpace(*m.Result) != "" {
				st.resultErr = strings.TrimSpace(*m.Result)
			}
		}

		if m.Result != nil && strings.TrimSpace(*m.Result) != "" {
			st.resultText = *m.Result
			st.resultChars = len(*m.Result)
		}

		out := st.bestOutput()
		if out != "" {
			return out, true, nil
		}

		if st.resultErr != "" {
			return "", true, &ProviderDebugError{
				Cause: fmt.Errorf("provider result error: %s", st.resultErr),
				Info:  debugInfo(),
			}
		}

		if st.assistantErr != "" {
			return "", true, &ProviderDebugError{
				Cause: fmt.Errorf("provider assistant error: %s", st.assistantErr),
				Info:  debugInfo(),
			}
		}

		return "", true, &ProviderDebugError{
			Cause: fmt.Errorf("empty provider response"),
			Info:  debugInfo(),
		}
	}

	return "", false, nil
}

func (st *askStreamState) bestOutput() string {
	if strings.TrimSpace(st.resultText) != "" {
		return st.resultText
	}

	out := strings.TrimSpace(st.assistantText.String())
	if out != "" {
		return out
	}

	return strings.TrimSpace(st.streamText.String())
}

func (st *askStreamState) finalResponse(debugInfo func() map[string]any) (string, error) {
	out := st.bestOutput()
	if out != "" {
		return out, nil
	}

	if st.resultErr != "" {
		return "", &ProviderDebugError{
			Cause: fmt.Errorf("provider result error: %s", st.resultErr),
			Info:  debugInfo(),
		}
	}

	if st.assistantErr != "" {
		return "", &ProviderDebugError{
			Cause: fmt.Errorf("provider assistant error: %s", st.assistantErr),
			Info:  debugInfo(),
		}
	}

	return "", &ProviderDebugError{
		Cause: fmt.Errorf("empty provider response"),
		Info:  debugInfo(),
	}
}

func streamEventChunks(
	msg *streamEvent,
	seq int,
	st *askStreamState,
	onChunk func(StreamChunk),
) int {
	if onChunk == nil {
		return 0
	}

	parts, ok := parseStreamEvent(msg)
	if !ok {
		return 0
	}

	// Track tool state across content blocks.
	if parts.ToolName != "" {
		st.currentToolName = parts.ToolName
		st.toolInputBuf.Reset()
	}

	if parts.ToolInputDelta != "" {
		st.toolInputBuf.WriteString(parts.ToolInputDelta)
	}

	n := 0

	// On content_block_stop, emit a rich tool summary if ending a tool block.
	if parts.EventType == "content_block_stop" && st.currentToolName != "" {
		summary := formatToolSummary(st.currentToolName, st.toolInputBuf.String())
		st.currentToolName = ""

		st.toolInputBuf.Reset()

		n++
		onChunk(StreamChunk{
			Kind:      StreamChunkMeta,
			Text:      summary,
			EventType: parts.EventType,
			Seq:       seq + n,
		})
	}

	if parts.Thinking != "" {
		n++
		onChunk(StreamChunk{
			Kind:      StreamChunkThinking,
			Text:      parts.Thinking,
			EventType: parts.EventType,
			Seq:       seq + n,
		})
	}

	if parts.Text != "" {
		n++
		onChunk(StreamChunk{
			Kind:      StreamChunkAnswer,
			Text:      parts.Text,
			EventType: parts.EventType,
			Seq:       seq + n,
		})
	}

	if parts.Meta != "" {
		n++
		onChunk(StreamChunk{
			Kind:      StreamChunkMeta,
			Text:      parts.Meta,
			EventType: parts.EventType,
			Seq:       seq + n,
		})
	}

	return n
}

type streamEventParts struct {
	EventType      string
	Thinking       string
	Text           string
	Meta           string
	ToolName       string
	ToolInputDelta string
}

const toolSummaryMaxLen = 120

func parseStreamEvent(msg *streamEvent) (streamEventParts, bool) {
	eventType, _ := msg.Event["type"].(string)
	parts := streamEventParts{
		EventType: eventType,
	}

	switch eventType {
	case "content_block_delta":
		delta, ok := msg.Event["delta"].(map[string]any)
		if !ok {
			return streamEventParts{}, false
		}

		if thinking, ok := delta["thinking"].(string); ok {
			parts.Thinking = thinking
		}

		if text, ok := delta["text"].(string); ok {
			parts.Text = text
		}

		if deltaType, _ := delta["type"].(string); deltaType == "input_json_delta" {
			if partial, ok := delta["partial_json"].(string); ok {
				parts.ToolInputDelta = partial
			}
		}
	case "content_block_start":
		contentBlock, ok := msg.Event["content_block"].(map[string]any)
		if !ok {
			return streamEventParts{}, false
		}

		blockType, _ := contentBlock["type"].(string)
		if text, ok := contentBlock["text"].(string); ok {
			parts.Text = text
		}

		if blockType == "tool_use" {
			toolName, _ := contentBlock["name"].(string)
			if strings.TrimSpace(toolName) == "" {
				toolName = "unknown"
			}

			parts.Meta = fmt.Sprintf("Using tool: %s", toolName)
			parts.ToolName = toolName
		}
	case "content_block_stop":
		// No meta emitted here; streamEventChunks handles tool
		// summaries via state, and non-tool blocks emit nothing.
	default:
		return streamEventParts{}, false
	}

	return parts, true
}

// formatToolSummary builds a human-readable summary of a completed tool
// invocation from the tool name and accumulated input JSON.
func formatToolSummary(toolName, inputJSON string) string {
	if strings.TrimSpace(inputJSON) == "" {
		return fmt.Sprintf("%s complete", toolName)
	}

	var fields map[string]any
	if err := json.Unmarshal([]byte(inputJSON), &fields); err != nil {
		return fmt.Sprintf("%s complete", toolName)
	}

	lower := strings.ToLower(toolName)

	switch lower {
	case "bash":
		if cmd, ok := fields["command"].(string); ok && cmd != "" {
			if len(cmd) > toolSummaryMaxLen {
				cmd = cmd[:toolSummaryMaxLen] + "..."
			}

			return fmt.Sprintf("Bash: %s", cmd)
		}
	case "read":
		if fp, ok := fields["file_path"].(string); ok && fp != "" {
			return fmt.Sprintf("Read: %s", fp)
		}
	case "write":
		if fp, ok := fields["file_path"].(string); ok && fp != "" {
			return fmt.Sprintf("Write: %s", fp)
		}
	case "edit":
		if fp, ok := fields["file_path"].(string); ok && fp != "" {
			return fmt.Sprintf("Edit: %s", fp)
		}
	case "glob":
		if pattern, ok := fields["pattern"].(string); ok && pattern != "" {
			return fmt.Sprintf("Glob: %s", pattern)
		}
	case "grep":
		if pattern, ok := fields["pattern"].(string); ok && pattern != "" {
			return fmt.Sprintf("Grep: %s", pattern)
		}
	}

	return fmt.Sprintf("%s complete", toolName)
}
