package ai

import (
	"encoding/json"
	"errors"
	"fmt"
)

const (
	// fieldType and blockTypeText are keys/values in the CLI wire protocol.
	fieldType     = "type"
	blockTypeText = "text"
)

// errUnhandledMessage marks a protocol line this package does not consume, such
// as system init notices and control responses.
var errUnhandledMessage = errors.New("unhandled provider message")

// The Claude Code CLI emits one JSON object per line when run with
// --output-format stream-json. The types below model only the subset of that
// protocol this package consumes; unknown line types decode to nil and are
// skipped by callers.

// textBlock is a text content block inside an assistant message.
type textBlock struct {
	Text string
}

// assistantMessage is a complete assistant turn emitted by the CLI.
type assistantMessage struct {
	Content []*textBlock
	Error   *string
}

// resultMessage terminates a turn and carries the final response.
type resultMessage struct {
	Subtype string
	IsError bool
	Result  *string
}

// streamEvent wraps a raw partial-message event, emitted when the CLI runs with
// --include-partial-messages. Event holds the underlying Anthropic API event.
type streamEvent struct {
	Event map[string]any
}

// decodeMessage converts a single stream-json line into one of the message
// types above. It returns nil for line types this package ignores, such as
// system init notices and control responses.
func decodeMessage(line []byte) (any, error) {
	//nolint:tagliatelle // Field names are fixed by the CLI wire protocol.
	var envelope struct {
		Type    string         `json:"type"`
		Subtype string         `json:"subtype"`
		IsError bool           `json:"is_error"`
		Result  *string        `json:"result"`
		Error   *string        `json:"error"`
		Event   map[string]any `json:"event"`
		Message struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}

	if err := json.Unmarshal(line, &envelope); err != nil {
		return nil, fmt.Errorf("decode provider message: %w", err)
	}

	switch envelope.Type {
	case "stream_event":
		return &streamEvent{Event: envelope.Event}, nil
	case "assistant":
		msg := &assistantMessage{Error: envelope.Error}

		for _, block := range envelope.Message.Content {
			if block.Type != blockTypeText {
				continue
			}

			msg.Content = append(msg.Content, &textBlock{Text: block.Text})
		}

		return msg, nil
	case "result":
		return &resultMessage{
			Subtype: envelope.Subtype,
			IsError: envelope.IsError,
			Result:  envelope.Result,
		}, nil
	default:
		return nil, errUnhandledMessage
	}
}

// encodeUserMessage builds a stream-json input line carrying a user prompt.
func encodeUserMessage(prompt string) ([]byte, error) {
	payload := map[string]any{
		fieldType: "user",
		"message": map[string]any{
			"role":    "user",
			"content": []map[string]any{{fieldType: blockTypeText, blockTypeText: prompt}},
		},
	}

	return json.Marshal(payload)
}

// encodeInterrupt builds a control request that aborts the in-flight turn.
func encodeInterrupt(requestID string) ([]byte, error) {
	payload := map[string]any{
		fieldType:    "control_request",
		"request_id": requestID,
		"request":    map[string]any{"subtype": "interrupt"},
	}

	return json.Marshal(payload)
}
