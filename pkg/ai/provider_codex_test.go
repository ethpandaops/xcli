package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCodexEngine_Provider(t *testing.T) {
	t.Parallel()

	engine := newCodexEngine(testLogger())
	assert.Equal(t, ProviderCodex, engine.Provider())
}

func TestCodexEngine_Capabilities(t *testing.T) {
	t.Parallel()

	engine := newCodexEngine(testLogger())
	caps := engine.Capabilities()

	assert.True(t, caps.Streaming)
	assert.True(t, caps.Interrupt)
	assert.True(t, caps.Sessions)
}
