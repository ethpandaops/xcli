// Package diagnostic provides types and utilities for capturing and analyzing
// rebuild operation outcomes in xcli.
package diagnostic

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// BuildPhase represents a stage in the build pipeline.
type BuildPhase string

const (
	// PhaseProtoGen represents the protocol buffer generation phase.
	PhaseProtoGen BuildPhase = "proto-gen"
	// PhaseBuild represents the main build/compilation phase.
	PhaseBuild BuildPhase = "build"
	// PhaseConfigGen represents the configuration generation phase.
	PhaseConfigGen BuildPhase = "config-gen"
	// PhaseRestart represents the service restart phase.
	PhaseRestart BuildPhase = "restart"
	// PhaseFrontendGen represents the frontend generation phase.
	PhaseFrontendGen BuildPhase = "frontend-gen"
)

// BuildResult captures the outcome of a single build operation.
type BuildResult struct {
	// Phase identifies which stage of the build pipeline this result is from.
	Phase BuildPhase `json:"phase"`
	// Service is the name of the service being built.
	Service string `json:"service"`
	// Command is the full command that was executed.
	Command string `json:"command"`
	// WorkDir is the working directory where the command was executed.
	WorkDir string `json:"workDir"`
	// Success indicates whether the build operation completed successfully.
	Success bool `json:"success"`
	// Duration is how long the build operation took.
	Duration time.Duration `json:"duration"`
	// Error is the error that occurred, if any. Not serialized to JSON.
	Error error `json:"-"`
	// ErrorMsg is the string representation of the error for JSON serialization.
	ErrorMsg string `json:"errorMsg"`
	// Stdout contains the standard output from the command.
	Stdout string `json:"stdout"`
	// Stderr contains the standard error output from the command.
	Stderr string `json:"stderr"`
	// ExitCode is the exit code returned by the command.
	ExitCode int `json:"exitCode"`
	// StartTime is when the build operation started.
	StartTime time.Time `json:"startTime"`
	// EndTime is when the build operation completed.
	EndTime time.Time `json:"endTime"`
}

// RebuildReport aggregates all build results for a single rebuild operation.
type RebuildReport struct {
	// ID is a unique identifier for this rebuild report.
	ID string `json:"id"`
	// StartTime is when the rebuild operation started.
	StartTime time.Time `json:"startTime"`
	// EndTime is when the rebuild operation completed.
	EndTime time.Time `json:"endTime"`
	// Duration is the total time taken for the rebuild operation.
	Duration time.Duration `json:"duration"`
	// Results contains all individual build results.
	Results []BuildResult `json:"results"`
	// Success indicates whether all build operations completed successfully.
	Success bool `json:"success"`
	// FailedCount is the number of build operations that failed.
	FailedCount int `json:"failedCount"`
	// TotalCount is the total number of build operations executed.
	TotalCount int `json:"totalCount"`
}

// NewRebuildReport creates a new report with a generated ID and initialized fields.
func NewRebuildReport() *RebuildReport {
	return &RebuildReport{
		ID:        generateID(),
		StartTime: time.Now(),
		Results:   make([]BuildResult, 0, 10),
	}
}

// AddResult adds a build result to the report.
func (r *RebuildReport) AddResult(result BuildResult) {
	r.Results = append(r.Results, result)
}

// Failed returns all failed build results.
func (r *RebuildReport) Failed() []BuildResult {
	failed := make([]BuildResult, 0, len(r.Results))

	for _, result := range r.Results {
		if !result.Success {
			failed = append(failed, result)
		}
	}

	return failed
}

// Succeeded returns all successful build results.
func (r *RebuildReport) Succeeded() []BuildResult {
	succeeded := make([]BuildResult, 0, len(r.Results))

	for _, result := range r.Results {
		if result.Success {
			succeeded = append(succeeded, result)
		}
	}

	return succeeded
}

// HasFailures returns true if any step failed.
func (r *RebuildReport) HasFailures() bool {
	for _, result := range r.Results {
		if !result.Success {
			return true
		}
	}

	return false
}

// FirstError returns the first failed result, or nil if all succeeded.
func (r *RebuildReport) FirstError() *BuildResult {
	for i := range r.Results {
		if !r.Results[i].Success {
			return &r.Results[i]
		}
	}

	return nil
}

// Finalize calculates summary statistics and marks the report as complete.
// It sets EndTime, Duration, Success, FailedCount, and TotalCount.
func (r *RebuildReport) Finalize() {
	r.EndTime = time.Now()
	r.Duration = r.EndTime.Sub(r.StartTime)
	r.TotalCount = len(r.Results)
	r.FailedCount = len(r.Failed())
	r.Success = r.FailedCount == 0
}

// generateID creates a random 16-character hex string using crypto/rand.
func generateID() string {
	bytes := make([]byte, 8)

	if _, err := rand.Read(bytes); err != nil {
		// Fall back to timestamp-based ID if crypto/rand fails.
		// This should be extremely rare in practice.
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405")))[:16]
	}

	return hex.EncodeToString(bytes)
}
