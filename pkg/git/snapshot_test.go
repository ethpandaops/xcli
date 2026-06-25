package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSnapshotReadsBranchCommitAndDirtyState(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	ctx := context.Background()
	repoDir := t.TempDir()

	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	runGit(t, repoDir, "config", "user.name", "Test User")
	runGit(t, repoDir, "config", "commit.gpgsign", "false")

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello\n"), 0644))
	runGit(t, repoDir, "add", "README.md")
	runGit(t, repoDir, "commit", "-m", "initial")

	version, err := Snapshot(ctx, repoDir)
	require.NoError(t, err)
	require.Equal(t, repoDir, version.Path)
	require.NotEmpty(t, version.Branch)
	require.Len(t, version.Commit, 40)
	require.False(t, version.Dirty)

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "dirty.txt"), []byte("dirty\n"), 0644))

	version, err = Snapshot(ctx, repoDir)
	require.NoError(t, err)
	require.True(t, version.Dirty)
}

func runGit(t *testing.T, repoDir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = repoDir

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, strings.TrimSpace(string(out)))
}
