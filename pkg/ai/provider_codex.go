package ai

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	codexsdk "github.com/wagiedev/codex-agent-sdk-go"
)

type codexEngine struct {
	log       logrus.FieldLogger
	timeout   time.Duration
	available bool
}

func newCodexEngine(log logrus.FieldLogger) *codexEngine {
	_, err := exec.LookPath("codex")

	return &codexEngine{
		log:       log.WithField("component", "ai-provider-codex"),
		timeout:   defaultTimeout,
		available: err == nil,
	}
}

func (e *codexEngine) Provider() ProviderID {
	return ProviderCodex
}

func (e *codexEngine) Capabilities() Capabilities {
	return Capabilities{Streaming: true, Interrupt: true, Sessions: true}
}

func (e *codexEngine) IsAvailable() bool {
	return e.available
}

func (e *codexEngine) Ask(ctx context.Context, prompt string) (string, error) {
	if !e.available {
		return "", fmt.Errorf("provider %s is not available", e.Provider())
	}

	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	var (
		assistantText strings.Builder
		resultText    string
	)

	for msg, err := range codexsdk.Query(ctx, prompt) {
		if err != nil {
			return "", err
		}

		switch m := msg.(type) {
		case *codexsdk.AssistantMessage:
			for _, block := range m.Content {
				if textBlock, ok := block.(*codexsdk.TextBlock); ok &&
					strings.TrimSpace(textBlock.Text) != "" {
					if assistantText.Len() > 0 {
						assistantText.WriteString("\n")
					}

					assistantText.WriteString(textBlock.Text)
				}
			}
		case *codexsdk.ResultMessage:
			if m.Result != nil && strings.TrimSpace(*m.Result) != "" {
				resultText = *m.Result
			}
		}
	}

	if strings.TrimSpace(resultText) != "" {
		return resultText, nil
	}

	out := strings.TrimSpace(assistantText.String())
	if out == "" {
		return "", fmt.Errorf("empty provider response")
	}

	return out, nil
}

func (e *codexEngine) StartSession(ctx context.Context) (Session, error) {
	if !e.available {
		return nil, fmt.Errorf("provider %s is not available", e.Provider())
	}

	client := codexsdk.NewClient()
	sessionCtx, sessionCancel := context.WithCancel(context.Background())
	session := &codexSession{
		id:              newSessionID(),
		log:             e.log,
		client:          client,
		timeout:         e.timeout,
		sessionCancel:   sessionCancel,
		stderrLineLimit: 200,
	}

	startErrCh := make(chan error, 1)

	go func() {
		startErrCh <- client.Start(
			sessionCtx,
			codexsdk.WithPermissionMode("bypassPermissions"),
			codexsdk.WithStderr(session.pushStderr),
		)
	}()

	select {
	case err := <-startErrCh:
		if err != nil {
			sessionCancel()

			return nil, err
		}
	case <-ctx.Done():
		sessionCancel()

		return nil, ctx.Err()
	}

	return session, nil
}

type codexSession struct {
	id              string
	log             logrus.FieldLogger
	client          codexsdk.Client
	timeout         time.Duration
	sessionCancel   context.CancelFunc
	mu              sync.Mutex
	stderrMu        sync.Mutex
	stderrLines     []string
	stderrLineLimit int
	closed          bool
}

func (s *codexSession) ID() string {
	return s.id
}

func (s *codexSession) AskStream(
	ctx context.Context,
	prompt string,
	onChunk func(StreamChunk),
) (string, error) {
	if s.isClosed() {
		return "", fmt.Errorf("session is closed")
	}

	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	state := &askStreamState{}

	debugInfo := func() map[string]any {
		return state.debugInfo(
			s.id, len(prompt), int(s.timeout.Seconds()),
			s.stderrCount(), s.stderrTail(20),
		)
	}

	if err := s.client.Query(ctx, prompt); err != nil {
		return "", &ProviderDebugError{Cause: err, Info: debugInfo()}
	}

	for msg, err := range s.client.ReceiveResponse(ctx) {
		if err != nil {
			loopBreak, retErr := state.handleReceiveError(err, debugInfo)
			if retErr != nil {
				return "", retErr
			}

			if loopBreak {
				break
			}

			continue
		}

		if out, done, retErr := handleCodexMessage(state, msg, onChunk, debugInfo); done {
			if retErr != nil {
				return "", retErr
			}

			return out, nil
		}
	}

	return state.finalResponse(debugInfo)
}

// handleCodexMessage processes a single message from the Codex SDK stream.
func handleCodexMessage(
	st *askStreamState,
	msg any,
	onChunk func(StreamChunk),
	debugInfo func() map[string]any,
) (string, bool, error) {
	st.msgCount++

	switch m := msg.(type) {
	case *codexsdk.StreamEvent:
		st.lastMessageType = "stream_event"
		st.streamEventCount++

		if eventType, ok := m.Event["type"].(string); ok {
			st.lastEventType = eventType
		}

		st.seq += streamEventChunksFromMap(m.Event, st.seq, st, onChunk)

		if parts, ok := parseStreamEventMap(m.Event); ok {
			if parts.Thinking != "" {
				st.thinkingChars += len(parts.Thinking)
			}

			if parts.Text != "" {
				st.streamText.WriteString(parts.Text)
				st.streamChars += len(parts.Text)
			}
		}
	case *codexsdk.AssistantMessage:
		st.lastMessageType = "assistant"
		st.assistantCount++

		if m.Error != nil {
			st.assistantErr = string(*m.Error)
		}

		for _, block := range m.Content {
			textBlock, ok := block.(*codexsdk.TextBlock)
			if !ok || strings.TrimSpace(textBlock.Text) == "" {
				continue
			}

			if st.assistantText.Len() > 0 {
				st.assistantText.WriteString("\n")
			}

			st.assistantText.WriteString(textBlock.Text)
			st.assistantChars += len(textBlock.Text)

			// Some CLI/SDK runs only surface thinking in stream events and
			// provide answer text via assistant messages before final result.
			// Emit fallback answer chunks so the UI streams visible answer
			// text during the turn.
			if onChunk != nil && st.streamChars == 0 {
				st.seq++
				onChunk(StreamChunk{
					Kind:      StreamChunkAnswer,
					Text:      textBlock.Text,
					EventType: "assistant_message",
					Seq:       st.seq,
				})
			}
		}
	case *codexsdk.ResultMessage:
		return handleResultMessage(st, m.Subtype, m.IsError, m.Result, debugInfo)
	}

	return "", false, nil
}

func (s *codexSession) Interrupt(ctx context.Context) error {
	if s.isClosed() {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	return s.client.Interrupt(ctx)
}

func (s *codexSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	if s.sessionCancel != nil {
		s.sessionCancel()
	}

	return s.client.Close()
}

func (s *codexSession) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.closed
}

func (s *codexSession) pushStderr(line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}

	s.stderrMu.Lock()
	defer s.stderrMu.Unlock()

	s.stderrLines = append(s.stderrLines, trimmed)
	if s.stderrLineLimit <= 0 {
		return
	}

	if len(s.stderrLines) > s.stderrLineLimit {
		s.stderrLines = s.stderrLines[len(s.stderrLines)-s.stderrLineLimit:]
	}
}

func (s *codexSession) stderrCount() int {
	s.stderrMu.Lock()
	defer s.stderrMu.Unlock()

	return len(s.stderrLines)
}

func (s *codexSession) stderrTail(n int) []string {
	s.stderrMu.Lock()
	defer s.stderrMu.Unlock()

	if n <= 0 || len(s.stderrLines) == 0 {
		return nil
	}

	if len(s.stderrLines) <= n {
		out := make([]string, len(s.stderrLines))
		copy(out, s.stderrLines)

		return out
	}

	out := make([]string, n)
	copy(out, s.stderrLines[len(s.stderrLines)-n:])

	return out
}
