package prerequisites

import (
	"context"

	"github.com/sirupsen/logrus"
)

// Compile-time interface check.
var _ Checker = (*checker)(nil)

// Type represents the type of prerequisite.
type Type string

const (
	// TypeFileCopy copies a file from source to destination.
	TypeFileCopy Type = "file_copy"
	// TypeCommand executes a shell command.
	TypeCommand Type = "command"
	// TypeDirectoryCheck validates directory existence.
	TypeDirectoryCheck Type = "directory_check"
)

// Prerequisite represents a single setup step.
type Prerequisite struct {
	Type        Type
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

// Repo defines all prerequisites for a repository.
type Repo struct {
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

// checker implements the Checker interface.
type checker struct {
	log  logrus.FieldLogger
	defs map[string]Repo
}
