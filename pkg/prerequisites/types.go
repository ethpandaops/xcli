package prerequisites

import (
	"context"

	"github.com/sirupsen/logrus"
)

// PrerequisiteType represents the type of prerequisite.
type PrerequisiteType string

const (
	PrerequisiteTypeFileCopy       PrerequisiteType = "file_copy"
	PrerequisiteTypeCommand        PrerequisiteType = "command"
	PrerequisiteTypeDirectoryCheck PrerequisiteType = "directory_check"
)

// Prerequisite represents a single setup step.
type Prerequisite struct {
	Type        PrerequisiteType
	Description string

	// For file_copy type
	SourcePath      string
	DestinationPath string

	// For command type
	Command    string
	Args       []string
	WorkingDir string

	// For directory_check type
	DirectoryPath string
	ShouldExist   bool

	// Conditional execution
	SkipIfExists string // Skip if this path exists
}

// RepoPrerequisites defines all prerequisites for a repository.
type RepoPrerequisites struct {
	RepoName      string
	Prerequisites []Prerequisite
}

// Checker interface for checking and running prerequisites.
type Checker interface {
	// Check validates if prerequisites are met for a repo.
	Check(ctx context.Context, repoPath string, repoName string) error

	// Run executes prerequisites for a repo.
	Run(ctx context.Context, repoPath string, repoName string) error

	// CheckAndRun checks and runs prerequisites if needed.
	CheckAndRun(ctx context.Context, repoPath string, repoName string) error
}

// Compile-time interface check.
var _ Checker = (*checker)(nil)

// checker implements the Checker interface.
type checker struct {
	log  logrus.FieldLogger
	defs map[string]RepoPrerequisites
}
