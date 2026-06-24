package instance

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/workspace"
)

const idHashLength = 16

var invalidDockerNameChars = regexp.MustCompile(`[^a-z0-9_-]+`)

// ResolveID returns the selected instance id. CLI override wins over config,
// then the deterministic config-root/hash id is used.
func ResolveID(ws *workspace.Workspace, labCfg *config.LabConfig, cliOverride string) (string, error) {
	override := strings.TrimSpace(cliOverride)
	if override == "" && labCfg != nil {
		override = strings.TrimSpace(labCfg.Instance.ID)
	}

	if override != "" {
		return SanitizeID(override)
	}

	if ws == nil {
		return "", fmt.Errorf("workspace is required to derive instance id")
	}

	sum := sha256.Sum256([]byte(ws.ConfigPath))
	hash := hex.EncodeToString(sum[:])[:idHashLength]

	return SanitizeID(fmt.Sprintf("%s-%s", filepath.Base(ws.RootDir), hash))
}

// SanitizeID converts arbitrary input into a docker-name-safe id.
func SanitizeID(id string) (string, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	id = invalidDockerNameChars.ReplaceAllString(id, "-")
	id = strings.Trim(id, "-_")

	if id == "" {
		return "", fmt.Errorf("instance id is empty after sanitization")
	}

	return id, nil
}
