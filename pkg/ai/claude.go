package ai

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"maps"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	defaultTimeout = 30 * time.Minute

	// claudeBinary is the Claude Code CLI this provider drives.
	claudeBinary = "claude"

	// maxLineBytes bounds a single stream-json line. Tool results and long
	// assistant turns comfortably exceed bufio.Scanner's default 64KiB.
	maxLineBytes = 16 << 20
)

// ProviderDebugError carries structured diagnostics for provider failures.
type ProviderDebugError struct {
	Cause error
	Info  map[string]any
}

func (e *ProviderDebugError) Error() string {
	if e == nil || e.Cause == nil {
		return "provider error"
	}

	return e.Cause.Error()
}

func (e *ProviderDebugError) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.Cause
}

func (e *ProviderDebugError) DebugInfo() map[string]any {
	if e == nil || len(e.Info) == 0 {
		return map[string]any{}
	}

	out := make(map[string]any, len(e.Info))
	maps.Copy(out, e.Info)

	return out
}

type claudeEngine struct {
	log       logrus.FieldLogger
	timeout   time.Duration
	available bool
}

func newClaudeEngine(log logrus.FieldLogger) *claudeEngine {
	_, err := exec.LookPath(claudeBinary)

	return &claudeEngine{
		log:       log.WithField("component", "ai-provider-claude"),
		timeout:   defaultTimeout,
		available: err == nil,
	}
}

func (e *claudeEngine) Provider() ProviderID {
	return ProviderClaude
}

func (e *claudeEngine) Capabilities() Capabilities {
	return Capabilities{Streaming: true, Interrupt: true, Sessions: true}
}

func (e *claudeEngine) IsAvailable() bool {
	return e.available
}

// Ask runs a single non-interactive turn and returns the final response.
func (e *claudeEngine) Ask(ctx context.Context, prompt string) (string, error) {
	if !e.available {
		return "", fmt.Errorf("provider %s is not available", e.Provider())
	}

	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, claudeBinary,
		"--print",
		"--output-format", "stream-json",
		"--verbose",
	)
	cmd.Stdin = strings.NewReader(prompt)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("open provider stdout: %w", err)
	}

	var stderr strings.Builder

	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start provider: %w", err)
	}

	var (
		assistantText strings.Builder
		resultText    string
		scanErr       error
	)

	scanner := newLineScanner(stdout)
	for scanner.Scan() {
		msg, err := decodeMessage(scanner.Bytes())
		if err != nil {
			continue
		}

		switch m := msg.(type) {
		case *assistantMessage:
			for _, block := range m.Content {
				if strings.TrimSpace(block.Text) == "" {
					continue
				}

				if assistantText.Len() > 0 {
					assistantText.WriteString("\n")
				}

				assistantText.WriteString(block.Text)
			}
		case *resultMessage:
			if m.Result != nil && strings.TrimSpace(*m.Result) != "" {
				resultText = *m.Result
			}
		}
	}

	if err := scanner.Err(); err != nil {
		scanErr = fmt.Errorf("read provider output: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("provider exited: %w: %s", err, tailString(stderr.String()))
	}

	if scanErr != nil {
		return "", scanErr
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

// StartSession launches a long-lived CLI process in streaming input mode, which
// keeps conversation state across turns.
func (e *claudeEngine) StartSession(ctx context.Context) (Session, error) {
	if !e.available {
		return nil, fmt.Errorf("provider %s is not available", e.Provider())
	}

	// The session owns this context for the lifetime of the child process, so
	// it deliberately outlives the caller's start context.
	sessionCtx, sessionCancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(sessionCtx, claudeBinary,
		"--print",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"--verbose",
		"--permission-mode", "bypassPermissions",
	)

	session := &claudeSession{
		id:              newSessionID(),
		log:             e.log,
		cmd:             cmd,
		timeout:         e.timeout,
		sessionCancel:   sessionCancel,
		messages:        make(chan sessionMessage, 64),
		stderrLineLimit: 200,
	}

	if err := session.start(ctx); err != nil {
		sessionCancel()

		return nil, err
	}

	return session, nil
}

// sessionMessage is one decoded protocol message, or a terminal read error.
type sessionMessage struct {
	msg any
	err error
}

type claudeSession struct {
	id            string
	log           logrus.FieldLogger
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	timeout       time.Duration
	sessionCancel context.CancelFunc

	messages chan sessionMessage
	wg       sync.WaitGroup

	mu       sync.Mutex
	stdinMu  sync.Mutex
	reqSeq   int
	closed   bool
	waitOnce sync.Once

	stderrMu        sync.Mutex
	stderrLines     []string
	stderrLineLimit int
}

// start wires up the child process pipes and launches the CLI. In streaming
// input mode the CLI stays silent until it receives a first prompt, so there is
// no readiness signal to wait for; a failed launch surfaces on the first turn.
func (s *claudeSession) start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	stdin, err := s.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("open provider stdin: %w", err)
	}

	stdout, err := s.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open provider stdout: %w", err)
	}

	stderr, err := s.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("open provider stderr: %w", err)
	}

	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("start provider: %w", err)
	}

	s.stdin = stdin

	s.wg.Add(2)

	go s.readStdout(stdout)
	go s.readStderr(stderr)

	return nil
}

// readStdout decodes protocol lines onto the message channel until the child
// process closes its stdout.
func (s *claudeSession) readStdout(stdout io.Reader) {
	defer s.wg.Done()
	defer close(s.messages)

	scanner := newLineScanner(stdout)
	for scanner.Scan() {
		msg, err := decodeMessage(scanner.Bytes())
		if err != nil {
			if !errors.Is(err, errUnhandledMessage) {
				s.log.WithError(err).Debug("skipping undecodable provider line")
			}

			continue
		}

		s.messages <- sessionMessage{msg: msg}
	}

	if err := scanner.Err(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		s.messages <- sessionMessage{err: fmt.Errorf("read provider output: %w", err)}
	}
}

func (s *claudeSession) readStderr(stderr io.Reader) {
	defer s.wg.Done()

	scanner := newLineScanner(stderr)
	for scanner.Scan() {
		s.pushStderr(scanner.Text())
	}
}

func (s *claudeSession) ID() string {
	return s.id
}

func (s *claudeSession) AskStream(ctx context.Context, prompt string, onChunk func(StreamChunk)) (string, error) {
	if s.isClosed() {
		return "", fmt.Errorf("session is closed")
	}

	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	state := &askStreamState{}

	debugInfo := func() map[string]any {
		return state.debugInfo(s, prompt)
	}

	line, err := encodeUserMessage(prompt)
	if err != nil {
		return "", &ProviderDebugError{Cause: err, Info: debugInfo()}
	}

	if err := s.writeLine(line); err != nil {
		return "", &ProviderDebugError{Cause: err, Info: debugInfo()}
	}

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case received, ok := <-s.messages:
			if !ok {
				state.endedWithEOF = true

				return state.finalResponse(debugInfo)
			}

			if received.err != nil {
				loopBreak, retErr := state.handleReceiveError(received.err, debugInfo)
				if retErr != nil {
					return "", retErr
				}

				if loopBreak {
					return state.finalResponse(debugInfo)
				}

				continue
			}

			if out, done, retErr := state.handleMessage(received.msg, onChunk, debugInfo); done {
				if retErr != nil {
					return "", retErr
				}

				return out, nil
			}
		}
	}
}

// Interrupt asks the CLI to abort the turn currently in flight.
func (s *claudeSession) Interrupt(_ context.Context) error {
	if s.isClosed() {
		return nil
	}

	s.mu.Lock()
	s.reqSeq++
	requestID := fmt.Sprintf("req_%d_%s", s.reqSeq, s.id)
	s.mu.Unlock()

	line, err := encodeInterrupt(requestID)
	if err != nil {
		return fmt.Errorf("encode interrupt: %w", err)
	}

	return s.writeLine(line)
}

func (s *claudeSession) Close() error {
	s.mu.Lock()

	if s.closed {
		s.mu.Unlock()

		return nil
	}

	s.closed = true
	s.mu.Unlock()

	if s.stdin != nil {
		// Closing stdin lets the CLI shut down cleanly before the context kill.
		_ = s.stdin.Close()
	}

	if s.sessionCancel != nil {
		s.sessionCancel()
	}

	s.wg.Wait()

	var err error

	s.waitOnce.Do(func() {
		if waitErr := s.cmd.Wait(); waitErr != nil && !isExpectedExit(waitErr) {
			err = fmt.Errorf("provider exited: %w", waitErr)
		}
	})

	return err
}

func (s *claudeSession) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.closed
}

func (s *claudeSession) writeLine(line []byte) error {
	s.stdinMu.Lock()
	defer s.stdinMu.Unlock()

	if s.stdin == nil {
		return fmt.Errorf("session stdin is not available")
	}

	if _, err := s.stdin.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write to provider: %w", err)
	}

	return nil
}

func (s *claudeSession) pushStderr(line string) {
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

func (s *claudeSession) stderrCount() int {
	s.stderrMu.Lock()
	defer s.stderrMu.Unlock()

	return len(s.stderrLines)
}

func (s *claudeSession) stderrTail(n int) []string {
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

// newLineScanner returns a scanner sized for large protocol lines.
func newLineScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineBytes)

	return scanner
}

// isExpectedExit reports whether a process error is the result of the session
// tearing the child process down itself.
func isExpectedExit(err error) bool {
	if errors.Is(err, context.Canceled) {
		return true
	}

	var exitErr *exec.ExitError

	return errors.As(err, &exitErr)
}

// tailString trims provider stderr to a bounded, single-line snippet.
func tailString(s string) string {
	const maxLen = 512

	trimmed := strings.TrimSpace(s)
	if len(trimmed) > maxLen {
		trimmed = trimmed[len(trimmed)-maxLen:]
	}

	return trimmed
}

func newSessionID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("sess-%d", time.Now().UnixNano())
	}

	return "sess-" + hex.EncodeToString(buf)
}
