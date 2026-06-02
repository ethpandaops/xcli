package ai

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
)

// DefaultProvider is used when callers do not specify an explicit provider.
const DefaultProvider = ProviderClaude

// SupportedProviders lists selectable provider IDs.
func SupportedProviders() []ProviderID {
	return []ProviderID{ProviderClaude, ProviderCodex}
}

// NewEngine creates an Engine for the given provider.
func NewEngine(provider ProviderID, log logrus.FieldLogger) (Engine, error) {
	switch provider {
	case "", ProviderClaude:
		return newClaudeEngine(log), nil
	case ProviderCodex:
		return newCodexEngine(log), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

// ListProviderInfo returns metadata for all supported providers.
func ListProviderInfo(ctx context.Context, log logrus.FieldLogger, defaultProvider ProviderID) []ProviderInfo {
	providers := SupportedProviders()
	infos := make([]ProviderInfo, 0, len(providers))

	for _, provider := range providers {
		engine, err := NewEngine(provider, log)
		if err != nil {
			infos = append(infos, ProviderInfo{
				ID:           provider,
				Label:        providerLabel(provider),
				Default:      provider == defaultProvider,
				Available:    false,
				Capabilities: providerCapabilities(provider),
			})

			continue
		}

		infos = append(infos, ProviderInfo{
			ID:           provider,
			Label:        providerLabel(provider),
			Default:      provider == defaultProvider,
			Available:    engine.IsAvailable(),
			Capabilities: engine.Capabilities(),
		})
	}

	return infos
}

func providerLabel(provider ProviderID) string {
	switch provider {
	case ProviderClaude:
		return "Claude"
	case ProviderCodex:
		return "Codex"
	default:
		return string(provider)
	}
}

func providerCapabilities(provider ProviderID) Capabilities {
	switch provider {
	case ProviderClaude:
		return Capabilities{Streaming: true, Interrupt: true, Sessions: true}
	case ProviderCodex:
		return Capabilities{Streaming: true, Interrupt: true, Sessions: true}
	default:
		return Capabilities{}
	}
}
