// Package compose provides a reusable docker-compose command runner
// for managing container stacks defined by docker-compose.yml files.
package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

// ServiceStatus represents the status of a single docker-compose service.
type ServiceStatus struct {
	Name    string `json:"name"`
	Service string `json:"service"`
	State   string `json:"state"`
	Status  string `json:"status"`
	Ports   string `json:"ports"`
}

// Runner executes docker-compose commands against a project directory.
type Runner struct {
	log          logrus.FieldLogger
	projectDir   string
	profiles     []string
	envOverrides map[string]string
}

// NewRunner creates a new docker-compose Runner.
// It validates that the project directory contains a docker-compose.yml file.
func NewRunner(
	log logrus.FieldLogger,
	projectDir string,
	profiles []string,
	envOverrides map[string]string,
) (*Runner, error) {
	absDir, err := filepath.Abs(projectDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve project directory: %w", err)
	}

	composePath := filepath.Join(absDir, "docker-compose.yml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("docker-compose.yml not found in %s", absDir)
	}

	return &Runner{
		log:          log,
		projectDir:   absDir,
		profiles:     profiles,
		envOverrides: envOverrides,
	}, nil
}

// Up starts the docker-compose stack.
func (r *Runner) Up(ctx context.Context, build bool, services ...string) error {
	args := []string{"up", "-d"}
	if build {
		args = append(args, "--build")
	}

	args = append(args, services...)

	return r.run(ctx, args...)
}

// Down stops the docker-compose stack.
func (r *Runner) Down(ctx context.Context, volumes bool, removeImages bool) error {
	args := []string{"down"}
	if volumes {
		args = append(args, "-v")
	}

	if removeImages {
		args = append(args, "--rmi", "all")
	}

	return r.run(ctx, args...)
}

// Start starts one or more stopped services.
func (r *Runner) Start(ctx context.Context, service string) error {
	return r.run(ctx, "start", service)
}

// Stop stops one or more running services.
func (r *Runner) Stop(ctx context.Context, service string) error {
	return r.run(ctx, "stop", service)
}

// Restart restarts one or more services.
func (r *Runner) Restart(ctx context.Context, service string) error {
	return r.run(ctx, "restart", service)
}

// Pull pulls the latest images for services.
func (r *Runner) Pull(ctx context.Context, services ...string) error {
	args := make([]string, 1, 1+len(services))
	args[0] = "pull"
	args = append(args, services...)

	return r.run(ctx, args...)
}

// Build builds or rebuilds service images.
func (r *Runner) Build(ctx context.Context, services ...string) error {
	args := make([]string, 1, 1+len(services))
	args[0] = "build"
	args = append(args, services...)

	return r.run(ctx, args...)
}

// Logs streams logs from services, piping stdout/stderr directly to the terminal.
func (r *Runner) Logs(ctx context.Context, service string, follow bool) error {
	args := []string{"logs"}
	if follow {
		args = append(args, "-f")
	}

	if service != "" {
		args = append(args, service)
	}

	return r.runAttached(ctx, args...)
}

// PS returns the status of all services in the stack.
func (r *Runner) PS(ctx context.Context) ([]ServiceStatus, error) {
	cmd := r.buildCommand(ctx, "ps", "--format", "json")

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker compose ps failed: %w (stderr: %s)", err, stderr.String())
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return nil, nil
	}

	// docker compose ps --format json outputs one JSON object per line
	var statuses []ServiceStatus

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var status ServiceStatus
		if err := json.Unmarshal([]byte(line), &status); err != nil {
			r.log.WithError(err).WithField("line", line).Warn("failed to parse service status")

			continue
		}

		statuses = append(statuses, status)
	}

	return statuses, nil
}

// ListServices returns all service names defined in the docker-compose configuration.
func (r *Runner) ListServices(ctx context.Context) ([]string, error) {
	cmd := r.buildCommand(ctx, "config", "--services")

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker compose config --services failed: %w (stderr: %s)", err, stderr.String())
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return nil, nil
	}

	lines := strings.Split(output, "\n")
	services := make([]string, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			services = append(services, line)
		}
	}

	return services, nil
}

// run executes a docker-compose command and streams output to the terminal.
func (r *Runner) run(ctx context.Context, args ...string) error {
	cmd := r.buildCommand(ctx, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	r.log.WithField("args", args).Debug("running docker compose command")

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose %s failed: %w", args[0], err)
	}

	return nil
}

// runAttached executes a docker-compose command with stdin/stdout/stderr
// connected directly to the terminal for interactive use (e.g., logs -f).
func (r *Runner) runAttached(ctx context.Context, args ...string) error {
	cmd := r.buildCommand(ctx, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	r.log.WithField("args", args).Debug("running attached docker compose command")

	err := cmd.Run()
	if err != nil {
		// Context cancellation is expected for follow mode
		if ctx.Err() != nil {
			return nil //nolint:nilerr // intentional: cancelled context is not an error
		}

		return fmt.Errorf("docker compose %s failed: %w", args[0], err)
	}

	return nil
}

// buildCommand creates an exec.Cmd for a docker compose invocation
// with the configured project directory, profiles, and environment overrides.
func (r *Runner) buildCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmdArgs := make([]string, 1, 1+2*len(r.profiles)+len(args))
	cmdArgs[0] = "compose"

	// Add profile flags
	for _, profile := range r.profiles {
		cmdArgs = append(cmdArgs, "--profile", profile)
	}

	cmdArgs = append(cmdArgs, args...)

	cmd := exec.CommandContext(ctx, "docker", cmdArgs...)
	cmd.Dir = r.projectDir

	// Build environment: inherit current env, enable BuildKit, then apply overrides
	env := os.Environ()
	env = append(env, "DOCKER_BUILDKIT=1")

	for k, v := range r.envOverrides {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	cmd.Env = env

	return cmd
}
