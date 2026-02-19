package tui

import (
	"bufio"
	"context"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/acarl005/stripansi"
)

// LogLine represents a parsed log line.
type LogLine struct {
	Service   string
	Timestamp time.Time
	Level     string
	Message   string
	Raw       string
}

// LogStreamer streams logs from multiple services.
type LogStreamer struct {
	mu       sync.Mutex
	services map[string]context.CancelFunc
	output   chan LogLine
}

// NewLogStreamer creates a log streamer.
func NewLogStreamer() *LogStreamer {
	return &LogStreamer{
		services: make(map[string]context.CancelFunc, 10),
		output:   make(chan LogLine, 10000), // Large buffer for high-volume logs
	}
}

// Start begins tailing logs for a service.
func (ls *LogStreamer) Start(serviceName, logFile string) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	// Check if already tailing this service
	if _, exists := ls.services[serviceName]; exists {
		return nil // Already streaming
	}

	ctx, cancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(ctx, "tail", "-f", "-n", "50", logFile)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()

		return err
	}

	if err := cmd.Start(); err != nil {
		cancel()

		return err
	}

	// Store cancel function for cleanup
	ls.services[serviceName] = cancel

	// Parse logs in background — removes itself from map when done
	go ls.streamLogs(ctx, serviceName, stdout)

	return nil
}

// StartDocker begins tailing Docker container logs for a service.
func (ls *LogStreamer) StartDocker(serviceName, containerName string) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	// Check if already tailing this service
	if _, exists := ls.services[serviceName]; exists {
		return nil // Already streaming
	}

	ctx, cancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(ctx, "docker", "logs", "-f", "--tail", "100", containerName)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()

		return err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()

		return err
	}

	if err := cmd.Start(); err != nil {
		cancel()

		return err
	}

	// Store cancel function for cleanup
	ls.services[serviceName] = cancel

	// Stream stdout and stderr concurrently — Docker containers may write to
	// either, and a sequential reader would block on the first until EOF.
	go func() {
		var wg sync.WaitGroup

		wg.Add(2) //nolint:mnd // stdout + stderr

		go func() {
			defer wg.Done()

			ls.streamPipe(ctx, serviceName, stdout)
		}()

		go func() {
			defer wg.Done()

			ls.streamPipe(ctx, serviceName, stderr)
		}()

		wg.Wait()

		// Both pipes done — remove from map so the service can be re-tailed
		ls.mu.Lock()
		delete(ls.services, serviceName)
		ls.mu.Unlock()
	}()

	return nil
}

// StopService stops log streaming for a single service.
// This kills the tail process and removes the entry so it can be re-started later.
func (ls *LogStreamer) StopService(serviceName string) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if cancel, exists := ls.services[serviceName]; exists {
		cancel()
		delete(ls.services, serviceName)
	}
}

// ActiveServices returns the names of services currently being tailed.
func (ls *LogStreamer) ActiveServices() []string {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	names := make([]string, 0, len(ls.services))
	for name := range ls.services {
		names = append(names, name)
	}

	return names
}

// Output returns the log line channel.
func (ls *LogStreamer) Output() <-chan LogLine {
	return ls.output
}

// Stop stops all log streaming.
func (ls *LogStreamer) Stop() {
	ls.mu.Lock()
	// Cancel all service contexts, which will kill the tail processes
	for name, cancel := range ls.services {
		cancel()
		delete(ls.services, name)
	}
	ls.mu.Unlock()

	close(ls.output)
}

// ParseLine parses a log line into structured format.
// Preserves full log content while extracting timestamp and level for display.
func ParseLine(service, raw string) LogLine {
	// Strip ANSI color codes
	stripped := stripansi.Strip(raw)

	line := LogLine{
		Service:   service,
		Raw:       raw,
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   stripped, // Keep full line content
	}

	// Try to extract timestamp from logrus format: time="2025-11-18T11:53:53-03:00"
	reTime := regexp.MustCompile(`time="([^"]+)"`)
	if matches := reTime.FindStringSubmatch(stripped); len(matches) == 2 {
		if t, err := time.Parse("2006-01-02T15:04:05-07:00", matches[1]); err == nil {
			line.Timestamp = t
		}
	}

	// Extract level from logrus format: level=warning or level=info
	reLevel := regexp.MustCompile(`level=(\w+)`)
	if matches := reLevel.FindStringSubmatch(stripped); len(matches) == 2 {
		line.Level = strings.ToUpper(matches[1])

		return line
	}

	// Format 1: [LEVEL][timestamp] message
	re1 := regexp.MustCompile(`\[([A-Z]+)\]\[([^\]]+)\]`)
	if matches := re1.FindStringSubmatch(stripped); len(matches) == 3 {
		line.Level = matches[1]
		if t, err := time.Parse("2006-01-02T15:04:05-07:00", matches[2]); err == nil {
			line.Timestamp = t
		}

		return line
	}

	// Extract level from anywhere in line as fallback
	upperStripped := strings.ToUpper(stripped)
	if strings.Contains(upperStripped, "ERROR") || strings.Contains(upperStripped, "ERRO") {
		line.Level = "ERROR"
	} else if strings.Contains(upperStripped, "WARN") {
		line.Level = "WARN"
	} else if strings.Contains(upperStripped, "DEBUG") {
		line.Level = "DEBUG"
	}

	return line
}

func (ls *LogStreamer) streamLogs(ctx context.Context, serviceName string, stdout any) {
	defer func() {
		// Remove from map so the service can be re-tailed after restart
		ls.mu.Lock()
		delete(ls.services, serviceName)
		ls.mu.Unlock()
	}()

	ls.streamPipe(ctx, serviceName, stdout)
}

// streamPipe reads lines from a pipe and sends them to the output channel.
// Does not manage the services map — caller is responsible for cleanup.
func (ls *LogStreamer) streamPipe(ctx context.Context, serviceName string, pipe any) {
	reader, ok := pipe.(interface{ Read([]byte) (int, error) })
	if !ok {
		return
	}

	scanner := bufio.NewScanner(reader)
	// Increase buffer size for long log lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := ParseLine(serviceName, scanner.Text())
		select {
		case ls.output <- line:
		case <-ctx.Done():
			return
		}
	}

	// If scanner exits, check for errors
	if err := scanner.Err(); err != nil {
		// Send error as a log line so it's visible
		errLine := LogLine{
			Service:   serviceName,
			Timestamp: time.Now(),
			Level:     "ERROR",
			Message:   "Log scanner error: " + err.Error(),
			Raw:       err.Error(),
		}
		select {
		case ls.output <- errLine:
		case <-ctx.Done():
		}
	}
}
