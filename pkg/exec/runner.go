// Package exec provides utilities for running external commands
// with consistent output handling and logging.
package exec

import (
	"bytes"
	"os"
	"os/exec"
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
