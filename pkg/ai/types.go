package ai

import "context"

// ProviderID identifies an AI provider implementation.
type ProviderID string

const (
	// ProviderClaude uses the Claude Code SDK-backed provider.
	ProviderClaude ProviderID = "claude"
)

// Capabilities describe what features a provider supports.
type Capabilities struct {
	Streaming bool `json:"streaming"`
	Interrupt bool `json:"interrupt"`
	Sessions  bool `json:"sessions"`
}

// ProviderInfo describes a configured/selectable provider.
type ProviderInfo struct {
	ID           ProviderID   `json:"id"`
	Label        string       `json:"label"`
	Default      bool         `json:"default"`
	Available    bool         `json:"available"`
	Capabilities Capabilities `json:"capabilities"`
}

// DiagnosisReport is the structured diagnosis shown in CLI/UI.
type DiagnosisReport struct {
	RootCause     string   `json:"rootCause"`
	Explanation   string   `json:"explanation"`
	AffectedFiles []string `json:"affectedFiles"`
	Suggestions   []string `json:"suggestions"`
	FixCommands   []string `json:"fixCommands,omitempty"`
}

// StreamChunkKind is the category of streamed output.
type StreamChunkKind string

const (
	StreamChunkThinking StreamChunkKind = "thinking"
	StreamChunkAnswer   StreamChunkKind = "answer"
	StreamChunkMeta     StreamChunkKind = "meta"
)

// StreamChunk is a normalized streamed event chunk.
type StreamChunk struct {
	Kind      StreamChunkKind `json:"kind"`
	Text      string          `json:"text"`
	EventType string          `json:"eventType,omitempty"`
	Seq       int             `json:"seq"`
}

// Session is a stateful provider conversation session.
type Session interface {
	ID() string
	AskStream(ctx context.Context, prompt string, onChunk func(StreamChunk)) (string, error)
	Interrupt(ctx context.Context) error
	Close() error
}

// Engine is a provider implementation selected at runtime.
type Engine interface {
	Provider() ProviderID
	Capabilities() Capabilities
	IsAvailable() bool
	Ask(ctx context.Context, prompt string) (string, error)
	StartSession(ctx context.Context) (Session, error)
}
