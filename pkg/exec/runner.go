// Package exec provides utilities for running external commands
// with consistent output handling and logging.
package exec

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ethpandaops/xcli/pkg/diagnostic"
)

// RunCmd runs a command with output handling based on verbose mode.
// In verbose mode, output is shown in real-time. In quiet mode, output
// is captured and only shown if the command fails.
func RunCmd(cmd *exec.Cmd, verbose bool) error {
	if verbose {
		// Verbose mode: show all output in real-time
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		return cmd.Run()
	}

	// Quiet mode: capture output, only show if command fails
	var output bytes.Buffer

	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Run(); err != nil {
		// Command failed - show captured output to help user understand what went wrong
		if output.Len() > 0 {
			os.Stderr.Write(output.Bytes())
		}

		return err
	}

	return nil
}

// RunCmdWithResult captures full output and returns BuildResult for diagnostics.
// This provides detailed information about the command execution including
// stdout, stderr, exit code, and timing information.
func RunCmdWithResult(
	cmd *exec.Cmd,
	verbose bool,
	phase diagnostic.BuildPhase,
	service string,
) *diagnostic.BuildResult {
	result := &diagnostic.BuildResult{
		Phase:     phase,
		Service:   service,
		Command:   strings.Join(cmd.Args, " "),
		WorkDir:   cmd.Dir,
		StartTime: time.Now(),
	}

	// Create buffers for capturing stdout and stderr separately
	var stdout, stderr bytes.Buffer

	if verbose {
		// Verbose mode: write to both console and buffers
		cmd.Stdout = io.MultiWriter(os.Stdout, &stdout)
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	} else {
		// Quiet mode: capture to buffers only
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	}

	// Run the command
	err := cmd.Run()

	// Record timing
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	// Capture output
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()

	if err != nil {
		// Command failed
		result.Success = false
		result.Error = err
		result.ErrorMsg = err.Error()

		// Extract exit code from ExitError
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}

		// In quiet mode, write output to stderr so user can see what went wrong
		if !verbose {
			if stdout.Len() > 0 {
				os.Stderr.Write(stdout.Bytes())
			}

			if stderr.Len() > 0 {
				os.Stderr.Write(stderr.Bytes())
			}
		}
	} else {
		// Command succeeded
		result.Success = true
		result.ExitCode = 0
	}

	return result
}
