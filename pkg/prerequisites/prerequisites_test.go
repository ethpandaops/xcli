package prerequisites

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteFileCopy(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(t *testing.T, repoPath string)
		prereq      Prerequisite
		expectError bool
	}{
		{
			name: "successful file copy",
			setupFunc: func(t *testing.T, repoPath string) {
				t.Helper()
				// Create source file
				err := os.WriteFile(filepath.Join(repoPath, "source.txt"), []byte("test content"), 0600)
				require.NoError(t, err)
			},
			prereq: Prerequisite{
				Type:            PrerequisiteTypeFileCopy,
				Description:     "Copy source to dest",
				SourcePath:      "source.txt",
				DestinationPath: "dest.txt",
			},
			expectError: false,
		},
		{
			name: "source file does not exist",
			setupFunc: func(t *testing.T, repoPath string) {
				t.Helper()
			},
			prereq: Prerequisite{
				Type:            PrerequisiteTypeFileCopy,
				Description:     "Copy missing file",
				SourcePath:      "missing.txt",
				DestinationPath: "dest.txt",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			repoPath := t.TempDir()

			// Setup
			tt.setupFunc(t, repoPath)

			// Create checker
			log := logrus.New()
			log.SetOutput(os.Stdout)
			checker := &checker{
				log:  log,
				defs: make(map[string]RepoPrerequisites),
			}

			// Execute
			err := checker.executeFileCopy(context.Background(), repoPath, tt.prereq)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify destination file exists with correct content
				destPath := filepath.Join(repoPath, tt.prereq.DestinationPath)
				content, err := os.ReadFile(destPath)
				require.NoError(t, err)
				assert.Equal(t, "test content", string(content))
			}
		})
	}
}

func TestExecuteFileCopySkipIfExists(t *testing.T) {
	// Create temp directory
	repoPath := t.TempDir()

	// Create source and destination files
	sourceContent := "source content"
	destContent := "existing dest content"

	err := os.WriteFile(filepath.Join(repoPath, "source.txt"), []byte(sourceContent), 0600)
	require.NoError(t, err)

	destPath := filepath.Join(repoPath, "dest.txt")
	err = os.WriteFile(destPath, []byte(destContent), 0600)
	require.NoError(t, err)

	// Create custom prerequisites for testing
	log := logrus.New()
	log.SetOutput(os.Stdout)
	testChecker := &checker{
		log: log.WithField("component", "prerequisites"),
		defs: map[string]RepoPrerequisites{
			"test-repo": {
				RepoName: "test-repo",
				Prerequisites: []Prerequisite{
					{
						Type:            PrerequisiteTypeFileCopy,
						Description:     "Copy source to dest",
						SourcePath:      "source.txt",
						DestinationPath: "dest.txt",
						SkipIfExists:    "dest.txt",
					},
				},
			},
		},
	}

	// Execute
	err = testChecker.Run(context.Background(), repoPath, "test-repo")
	require.NoError(t, err)

	// Verify destination was NOT overwritten (skip condition worked)
	content, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, destContent, string(content), "destination should not be overwritten when SkipIfExists is set")
}

func TestExecuteCommand(t *testing.T) {
	tests := []struct {
		name        string
		prereq      Prerequisite
		expectError bool
		validate    func(t *testing.T, repoPath string)
	}{
		{
			name: "successful command execution",
			prereq: Prerequisite{
				Type:        PrerequisiteTypeCommand,
				Description: "Create test file",
				Command:     "touch",
				Args:        []string{"test-output.txt"},
				WorkingDir:  ".",
			},
			expectError: false,
			validate: func(t *testing.T, repoPath string) {
				t.Helper()
				// Verify file was created
				_, err := os.Stat(filepath.Join(repoPath, "test-output.txt"))
				assert.NoError(t, err)
			},
		},
		{
			name: "command not found",
			prereq: Prerequisite{
				Type:        PrerequisiteTypeCommand,
				Description: "Run nonexistent command",
				Command:     "nonexistent-command-xyz",
				Args:        []string{},
				WorkingDir:  ".",
			},
			expectError: true,
			validate: func(t *testing.T, repoPath string) {
				t.Helper()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			repoPath := t.TempDir()

			// Create checker
			log := logrus.New()
			log.SetOutput(os.Stdout)
			checker := &checker{
				log:  log,
				defs: make(map[string]RepoPrerequisites),
			}

			// Execute
			err := checker.executeCommand(context.Background(), repoPath, tt.prereq)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				tt.validate(t, repoPath)
			}
		})
	}
}

func TestExecuteDirectoryCheck(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(t *testing.T, repoPath string)
		prereq      Prerequisite
		expectError bool
	}{
		{
			name: "directory exists when it should",
			setupFunc: func(t *testing.T, repoPath string) {
				t.Helper()

				err := os.Mkdir(filepath.Join(repoPath, "test-dir"), 0755)
				require.NoError(t, err)
			},
			prereq: Prerequisite{
				Type:          PrerequisiteTypeDirectoryCheck,
				Description:   "Check directory exists",
				DirectoryPath: "test-dir",
				ShouldExist:   true,
			},
			expectError: false,
		},
		{
			name: "directory missing when it should exist",
			setupFunc: func(t *testing.T, repoPath string) {
				t.Helper()
			},
			prereq: Prerequisite{
				Type:          PrerequisiteTypeDirectoryCheck,
				Description:   "Check directory exists",
				DirectoryPath: "missing-dir",
				ShouldExist:   true,
			},
			expectError: true,
		},
		{
			name: "directory missing when it should not exist",
			setupFunc: func(t *testing.T, repoPath string) {
				t.Helper()
			},
			prereq: Prerequisite{
				Type:          PrerequisiteTypeDirectoryCheck,
				Description:   "Check directory does not exist",
				DirectoryPath: "should-not-exist",
				ShouldExist:   false,
			},
			expectError: false,
		},
		{
			name: "directory exists when it should not",
			setupFunc: func(t *testing.T, repoPath string) {
				t.Helper()

				err := os.Mkdir(filepath.Join(repoPath, "unwanted-dir"), 0755)
				require.NoError(t, err)
			},
			prereq: Prerequisite{
				Type:          PrerequisiteTypeDirectoryCheck,
				Description:   "Check directory does not exist",
				DirectoryPath: "unwanted-dir",
				ShouldExist:   false,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			repoPath := t.TempDir()

			// Setup
			tt.setupFunc(t, repoPath)

			// Create checker
			log := logrus.New()
			log.SetOutput(os.Stdout)
			checker := &checker{
				log:  log,
				defs: make(map[string]RepoPrerequisites),
			}

			// Execute
			err := checker.executeDirectoryCheck(context.Background(), repoPath, tt.prereq)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCheckAndRun(t *testing.T) {
	// Create temp directory
	repoPath := t.TempDir()

	// Create source file
	err := os.WriteFile(filepath.Join(repoPath, "source.txt"), []byte("test content"), 0600)
	require.NoError(t, err)

	// Create custom checker for testing
	log := logrus.New()
	log.SetOutput(os.Stdout)
	testChecker := &checker{
		log: log.WithField("component", "prerequisites"),
		defs: map[string]RepoPrerequisites{
			"test-repo": {
				RepoName: "test-repo",
				Prerequisites: []Prerequisite{
					{
						Type:            PrerequisiteTypeFileCopy,
						Description:     "Copy source to dest",
						SourcePath:      "source.txt",
						DestinationPath: "dest.txt",
						SkipIfExists:    "dest.txt",
					},
				},
			},
		},
	}

	// First run - should execute prerequisite
	err = testChecker.CheckAndRun(context.Background(), repoPath, "test-repo")
	require.NoError(t, err)

	// Verify file was copied
	destPath := filepath.Join(repoPath, "dest.txt")
	content, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, "test content", string(content))

	// Second run - should skip (idempotent)
	err = testChecker.CheckAndRun(context.Background(), repoPath, "test-repo")
	require.NoError(t, err)

	// Verify file still exists with same content
	content, err = os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, "test content", string(content))
}

func TestCheck(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(t *testing.T, repoPath string)
		prereqs     RepoPrerequisites
		expectError bool
	}{
		{
			name: "all prerequisites met",
			setupFunc: func(t *testing.T, repoPath string) {
				t.Helper()

				err := os.WriteFile(filepath.Join(repoPath, "dest.txt"), []byte("content"), 0600)
				require.NoError(t, err)
			},
			prereqs: RepoPrerequisites{
				RepoName: "test-repo",
				Prerequisites: []Prerequisite{
					{
						Type:            PrerequisiteTypeFileCopy,
						Description:     "Copy file",
						SourcePath:      "source.txt",
						DestinationPath: "dest.txt",
					},
				},
			},
			expectError: false,
		},
		{
			name: "prerequisite not met",
			setupFunc: func(t *testing.T, repoPath string) {
				t.Helper()
			},
			prereqs: RepoPrerequisites{
				RepoName: "test-repo",
				Prerequisites: []Prerequisite{
					{
						Type:            PrerequisiteTypeFileCopy,
						Description:     "Copy file",
						SourcePath:      "source.txt",
						DestinationPath: "missing.txt",
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			repoPath := t.TempDir()

			// Setup
			tt.setupFunc(t, repoPath)

			// Create custom checker for testing
			log := logrus.New()
			log.SetOutput(os.Stdout)
			testChecker := &checker{
				log: log.WithField("component", "prerequisites"),
				defs: map[string]RepoPrerequisites{
					"test-repo": tt.prereqs,
				},
			}

			// Execute check
			err := testChecker.Check(context.Background(), repoPath, "test-repo")

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestKnownRepos(t *testing.T) {
	// Get known repo prerequisites
	defs := buildKnownRepoPrerequisites()

	// Test lab-backend
	labBackend, exists := defs["lab-backend"]
	require.True(t, exists, "lab-backend should have prerequisites defined")
	assert.Equal(t, "lab-backend", labBackend.RepoName)
	assert.Len(t, labBackend.Prerequisites, 1, "lab-backend should have 1 prerequisite")
	assert.Equal(t, PrerequisiteTypeFileCopy, labBackend.Prerequisites[0].Type)
	assert.Equal(t, ".env.example", labBackend.Prerequisites[0].SourcePath)
	assert.Equal(t, ".env", labBackend.Prerequisites[0].DestinationPath)

	// Test lab
	lab, exists := defs["lab"]
	require.True(t, exists, "lab should have prerequisites defined")
	assert.Equal(t, "lab", lab.RepoName)
	assert.Len(t, lab.Prerequisites, 3, "lab should have 3 prerequisites")

	// First prerequisite: file copy
	assert.Equal(t, PrerequisiteTypeFileCopy, lab.Prerequisites[0].Type)
	assert.Equal(t, ".env.example", lab.Prerequisites[0].SourcePath)
	assert.Equal(t, ".env", lab.Prerequisites[0].DestinationPath)

	// Second prerequisite: pnpm install
	assert.Equal(t, PrerequisiteTypeCommand, lab.Prerequisites[1].Type)
	assert.Equal(t, "pnpm", lab.Prerequisites[1].Command)
	assert.Equal(t, []string{"install"}, lab.Prerequisites[1].Args)
	assert.Equal(t, "node_modules", lab.Prerequisites[1].SkipIfExists)

	// Third prerequisite: pnpm build
	assert.Equal(t, PrerequisiteTypeCommand, lab.Prerequisites[2].Type)
	assert.Equal(t, "pnpm", lab.Prerequisites[2].Command)
	assert.Equal(t, []string{"run", "build"}, lab.Prerequisites[2].Args)
	assert.Equal(t, "dist", lab.Prerequisites[2].SkipIfExists)

	// Test cbt-api
	cbtAPI, exists := defs["cbt-api"]
	require.True(t, exists, "cbt-api should have prerequisites defined")
	assert.Equal(t, "cbt-api", cbtAPI.RepoName)
	assert.Len(t, cbtAPI.Prerequisites, 1, "cbt-api should have 1 prerequisite")
	assert.Equal(t, PrerequisiteTypeFileCopy, cbtAPI.Prerequisites[0].Type)
	assert.Equal(t, "config.example.yaml", cbtAPI.Prerequisites[0].SourcePath)
	assert.Equal(t, "config.yaml", cbtAPI.Prerequisites[0].DestinationPath)

	// Test xatu-cbt
	xatuCBT, exists := defs["xatu-cbt"]
	require.True(t, exists, "xatu-cbt should have prerequisites defined")
	assert.Equal(t, "xatu-cbt", xatuCBT.RepoName)
	assert.Len(t, xatuCBT.Prerequisites, 1, "xatu-cbt should have 1 prerequisite")
	assert.Equal(t, PrerequisiteTypeFileCopy, xatuCBT.Prerequisites[0].Type)
	assert.Equal(t, "example.env", xatuCBT.Prerequisites[0].SourcePath)
	assert.Equal(t, ".env", xatuCBT.Prerequisites[0].DestinationPath)

	// Test cbt (should not have prerequisites)
	_, exists = defs["cbt"]
	assert.False(t, exists, "cbt should not have prerequisites")
}

func TestNoPrerequisitesForUnknownRepo(t *testing.T) {
	// Create checker
	log := logrus.New()
	log.SetOutput(os.Stdout)
	checker := NewChecker(log)

	// Create temp directory
	repoPath := t.TempDir()

	// Run for unknown repo (should succeed with no-op)
	err := checker.Run(context.Background(), repoPath, "unknown-repo")
	assert.NoError(t, err)

	// Check for unknown repo (should succeed with no-op)
	err = checker.Check(context.Background(), repoPath, "unknown-repo")
	assert.NoError(t, err)
}
