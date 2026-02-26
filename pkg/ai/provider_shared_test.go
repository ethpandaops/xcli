package ai

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderDebugError(t *testing.T) {
	t.Parallel()

	t.Run("Error with cause", func(t *testing.T) {
		t.Parallel()

		cause := fmt.Errorf("connection timeout")
		pde := &ProviderDebugError{Cause: cause, Info: map[string]any{"key": "val"}}

		assert.Equal(t, "connection timeout", pde.Error())
	})

	t.Run("Error with nil cause", func(t *testing.T) {
		t.Parallel()

		pde := &ProviderDebugError{}
		assert.Equal(t, "provider error", pde.Error())
	})

	t.Run("Error on nil receiver", func(t *testing.T) {
		t.Parallel()

		var pde *ProviderDebugError
		assert.Equal(t, "provider error", pde.Error())
	})

	t.Run("Unwrap returns cause", func(t *testing.T) {
		t.Parallel()

		cause := fmt.Errorf("root cause")
		pde := &ProviderDebugError{Cause: cause}

		assert.ErrorIs(t, pde, cause)
		assert.Equal(t, cause, pde.Unwrap())
	})

	t.Run("Unwrap on nil receiver", func(t *testing.T) {
		t.Parallel()

		var pde *ProviderDebugError
		assert.Nil(t, pde.Unwrap())
	})

	t.Run("DebugInfo returns copy", func(t *testing.T) {
		t.Parallel()

		info := map[string]any{"session": "abc", "count": 42}
		pde := &ProviderDebugError{
			Cause: fmt.Errorf("err"),
			Info:  info,
		}

		result := pde.DebugInfo()
		assert.Equal(t, "abc", result["session"])
		assert.Equal(t, 42, result["count"])

		// Mutating result should not affect original.
		result["session"] = "modified"

		assert.Equal(t, "abc", pde.Info["session"])
	})

	t.Run("DebugInfo on nil receiver", func(t *testing.T) {
		t.Parallel()

		var pde *ProviderDebugError
		assert.Empty(t, pde.DebugInfo())
	})

	t.Run("DebugInfo with empty info", func(t *testing.T) {
		t.Parallel()

		pde := &ProviderDebugError{Cause: fmt.Errorf("err")}
		assert.Empty(t, pde.DebugInfo())
	})

	t.Run("errors.As compatibility", func(t *testing.T) {
		t.Parallel()

		cause := fmt.Errorf("inner")
		wrapped := fmt.Errorf("outer: %w", &ProviderDebugError{
			Cause: cause,
			Info:  map[string]any{"k": "v"},
		})

		var pde *ProviderDebugError
		require.True(t, errors.As(wrapped, &pde))
		assert.Equal(t, "inner", pde.Error())
		assert.Equal(t, "v", pde.DebugInfo()["k"])
	})
}

func TestAskStreamState_BestOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setup      func(st *askStreamState)
		wantOutput string
	}{
		{
			name:       "empty state returns empty",
			setup:      func(_ *askStreamState) {},
			wantOutput: "",
		},
		{
			name: "result text preferred over all",
			setup: func(st *askStreamState) {
				st.resultText = "result answer"
				st.assistantText.WriteString("assistant answer")
				st.streamText.WriteString("stream answer")
			},
			wantOutput: "result answer",
		},
		{
			name: "assistant text when no result",
			setup: func(st *askStreamState) {
				st.assistantText.WriteString("assistant answer")
				st.streamText.WriteString("stream answer")
			},
			wantOutput: "assistant answer",
		},
		{
			name: "stream text as last resort",
			setup: func(st *askStreamState) {
				st.streamText.WriteString("stream answer")
			},
			wantOutput: "stream answer",
		},
		{
			name: "whitespace-only result falls through",
			setup: func(st *askStreamState) {
				st.resultText = "   "
				st.assistantText.WriteString("assistant answer")
			},
			wantOutput: "assistant answer",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			st := &askStreamState{}
			tc.setup(st)
			assert.Equal(t, tc.wantOutput, st.bestOutput())
		})
	}
}

func TestParseStreamEventMap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		event     map[string]any
		wantOK    bool
		wantParts streamEventParts
	}{
		{
			name: "content_block_delta with thinking",
			event: map[string]any{
				"type": "content_block_delta",
				"delta": map[string]any{
					"thinking": "let me think...",
				},
			},
			wantOK: true,
			wantParts: streamEventParts{
				EventType: "content_block_delta",
				Thinking:  "let me think...",
			},
		},
		{
			name: "content_block_delta with text",
			event: map[string]any{
				"type": "content_block_delta",
				"delta": map[string]any{
					"text": "hello world",
				},
			},
			wantOK: true,
			wantParts: streamEventParts{
				EventType: "content_block_delta",
				Text:      "hello world",
			},
		},
		{
			name: "content_block_delta with tool input",
			event: map[string]any{
				"type": "content_block_delta",
				"delta": map[string]any{
					"type":         "input_json_delta",
					"partial_json": `{"command":`,
				},
			},
			wantOK: true,
			wantParts: streamEventParts{
				EventType:      "content_block_delta",
				ToolInputDelta: `{"command":`,
			},
		},
		{
			name: "content_block_delta missing delta",
			event: map[string]any{
				"type": "content_block_delta",
			},
			wantOK: false,
		},
		{
			name: "content_block_start with text",
			event: map[string]any{
				"type": "content_block_start",
				"content_block": map[string]any{
					"type": "text",
					"text": "start text",
				},
			},
			wantOK: true,
			wantParts: streamEventParts{
				EventType: "content_block_start",
				Text:      "start text",
			},
		},
		{
			name: "content_block_start with tool_use",
			event: map[string]any{
				"type": "content_block_start",
				"content_block": map[string]any{
					"type": "tool_use",
					"name": "Bash",
				},
			},
			wantOK: true,
			wantParts: streamEventParts{
				EventType: "content_block_start",
				Meta:      "Using tool: Bash",
				ToolName:  "Bash",
			},
		},
		{
			name: "content_block_start with empty tool name",
			event: map[string]any{
				"type": "content_block_start",
				"content_block": map[string]any{
					"type": "tool_use",
					"name": "",
				},
			},
			wantOK: true,
			wantParts: streamEventParts{
				EventType: "content_block_start",
				Meta:      "Using tool: unknown",
				ToolName:  "unknown",
			},
		},
		{
			name: "content_block_start missing content_block",
			event: map[string]any{
				"type": "content_block_start",
			},
			wantOK: false,
		},
		{
			name: "content_block_stop",
			event: map[string]any{
				"type": "content_block_stop",
			},
			wantOK: true,
			wantParts: streamEventParts{
				EventType: "content_block_stop",
			},
		},
		{
			name: "unknown event type",
			event: map[string]any{
				"type": "message_start",
			},
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			parts, ok := parseStreamEventMap(tc.event)
			assert.Equal(t, tc.wantOK, ok)

			if tc.wantOK {
				assert.Equal(t, tc.wantParts, parts)
			}
		})
	}
}

func TestFormatToolSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		toolName  string
		inputJSON string
		want      string
	}{
		{
			name:      "Bash with command",
			toolName:  "Bash",
			inputJSON: `{"command":"ls -la"}`,
			want:      "Bash: ls -la",
		},
		{
			name:      "Read with file_path",
			toolName:  "Read",
			inputJSON: `{"file_path":"/tmp/foo.go"}`,
			want:      "Read: /tmp/foo.go",
		},
		{
			name:      "Write with file_path",
			toolName:  "Write",
			inputJSON: `{"file_path":"/tmp/bar.go"}`,
			want:      "Write: /tmp/bar.go",
		},
		{
			name:      "Edit with file_path",
			toolName:  "Edit",
			inputJSON: `{"file_path":"/tmp/baz.go"}`,
			want:      "Edit: /tmp/baz.go",
		},
		{
			name:      "Glob with pattern",
			toolName:  "Glob",
			inputJSON: `{"pattern":"**/*.go"}`,
			want:      "Glob: **/*.go",
		},
		{
			name:      "Grep with pattern",
			toolName:  "Grep",
			inputJSON: `{"pattern":"TODO"}`,
			want:      "Grep: TODO",
		},
		{
			name:      "unknown tool",
			toolName:  "CustomTool",
			inputJSON: `{"data":"something"}`,
			want:      "CustomTool complete",
		},
		{
			name:      "empty input",
			toolName:  "Bash",
			inputJSON: "",
			want:      "Bash complete",
		},
		{
			name:      "invalid JSON",
			toolName:  "Bash",
			inputJSON: "not json{",
			want:      "Bash complete",
		},
		{
			name:     "Bash with long command truncated",
			toolName: "Bash",
			inputJSON: fmt.Sprintf(
				`{"command":"%s"}`,
				strings.Repeat("x", 200),
			),
			want: "Bash: " + strings.Repeat("x", toolSummaryMaxLen) + "...",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := formatToolSummary(tc.toolName, tc.inputJSON)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestNewSessionID(t *testing.T) {
	t.Parallel()

	id1 := newSessionID()
	id2 := newSessionID()

	assert.True(t, strings.HasPrefix(id1, "sess-"), "should have sess- prefix")
	assert.True(t, strings.HasPrefix(id2, "sess-"), "should have sess- prefix")
	assert.NotEqual(t, id1, id2, "IDs should be unique")
}
