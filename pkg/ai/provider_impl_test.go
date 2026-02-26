package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClaudeEngine_Provider(t *testing.T) {
	t.Parallel()

	engine := newClaudeEngine(testLogger())
	assert.Equal(t, ProviderClaude, engine.Provider())
}

func TestClaudeEngine_Capabilities(t *testing.T) {
	t.Parallel()

	engine := newClaudeEngine(testLogger())
	caps := engine.Capabilities()

	assert.True(t, caps.Streaming)
	assert.True(t, caps.Interrupt)
	assert.True(t, caps.Sessions)
}
