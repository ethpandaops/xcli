package ai

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() logrus.FieldLogger {
	l := logrus.New()
	l.SetLevel(logrus.FatalLevel)

	return l
}

func TestNewEngine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		provider   ProviderID
		wantType   string
		wantErr    bool
		errContain string
	}{
		{
			name:     "empty defaults to claude",
			provider: "",
			wantType: "*ai.claudeEngine",
		},
		{
			name:     "claude provider",
			provider: ProviderClaude,
			wantType: "*ai.claudeEngine",
		},
		{
			name:     "codex provider",
			provider: ProviderCodex,
			wantType: "*ai.codexEngine",
		},
		{
			name:       "unknown provider",
			provider:   "unknown",
			wantErr:    true,
			errContain: "unsupported provider",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			engine, err := NewEngine(tc.provider, testLogger())

			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errContain)
				assert.Nil(t, engine)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, engine)
			assert.IsType(t, engine, engine, "engine type mismatch")

			// Verify concrete type via provider ID.
			if tc.provider == "" || tc.provider == ProviderClaude {
				assert.Equal(t, ProviderClaude, engine.Provider())
			} else {
				assert.Equal(t, tc.provider, engine.Provider())
			}
		})
	}
}

func TestSupportedProviders(t *testing.T) {
	t.Parallel()

	providers := SupportedProviders()

	assert.Len(t, providers, 2)
	assert.Contains(t, providers, ProviderClaude)
	assert.Contains(t, providers, ProviderCodex)
}

func TestListProviderInfo(t *testing.T) {
	t.Parallel()

	infos := ListProviderInfo(context.Background(), testLogger(), ProviderClaude)

	require.Len(t, infos, 2)

	labels := make(map[ProviderID]string, len(infos))
	defaults := make(map[ProviderID]bool, len(infos))

	for _, info := range infos {
		labels[info.ID] = info.Label
		defaults[info.ID] = info.Default
		assert.True(t, info.Capabilities.Streaming, "%s should support streaming", info.ID)
		assert.True(t, info.Capabilities.Interrupt, "%s should support interrupt", info.ID)
		assert.True(t, info.Capabilities.Sessions, "%s should support sessions", info.ID)
	}

	assert.Equal(t, "Claude", labels[ProviderClaude])
	assert.Equal(t, "Codex", labels[ProviderCodex])
	assert.True(t, defaults[ProviderClaude])
	assert.False(t, defaults[ProviderCodex])
}
