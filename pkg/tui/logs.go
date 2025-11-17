package tui

import (
	"bufio"
	"context"
	"os/exec"
	"regexp"
	"strings"
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
	services map[string]*exec.Cmd
	output   chan LogLine
	cancel   context.CancelFunc
}

// NewLogStreamer creates a log streamer.
func NewLogStreamer() *LogStreamer {
	_, cancel := context.WithCancel(context.Background())

	return &LogStreamer{
		services: make(map[string]*exec.Cmd),
		output:   make(chan LogLine, 10000), // Large buffer for high-volume logs
		cancel:   cancel,
	}
}

// Start begins tailing logs for a service.
func (ls *LogStreamer) Start(serviceName, logFile string) error {
	// Check if already tailing this service
	if _, exists := ls.services[serviceName]; exists {
		return nil // Already streaming
	}

	cmd := exec.CommandContext(context.Background(), "tail", "-f", "-n", "50", logFile)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	ls.services[serviceName] = cmd

	// Parse logs in background
	go ls.streamLogs(context.Background(), serviceName, stdout)

	return nil
}

func (ls *LogStreamer) streamLogs(ctx context.Context, serviceName string, stdout any) {
	scanner := bufio.NewScanner(stdout.(interface{ Read([]byte) (int, error) }))
	// Increase buffer size for long log lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := parseLine(serviceName, scanner.Text())
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

// Output returns the log line channel.
func (ls *LogStreamer) Output() <-chan LogLine {
	return ls.output
}

// Stop stops all log streaming.
func (ls *LogStreamer) Stop() {
	ls.cancel()

	for _, cmd := range ls.services {
		if cmd.Process != nil {
			_ = cmd.Process.Kill() // Best effort
		}
	}

	close(ls.output)
}

// parseLine parses a log line into structured format.
func parseLine(service, raw string) LogLine {
	// Strip ANSI color codes
	stripped := stripansi.Strip(raw)

	line := LogLine{
		Service:   service,
		Raw:       raw,
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   stripped,
	}

	// Parse common log formats
	// Format 1: [LEVEL][timestamp] message
	re1 := regexp.MustCompile(`\[([A-Z]+)\]\[([^\]]+)\]\s*(.*)`)
	if matches := re1.FindStringSubmatch(stripped); len(matches) == 4 {
		line.Level = matches[1]
		if t, err := time.Parse("2006-01-02T15:04:05-07:00", matches[2]); err == nil {
			line.Timestamp = t
		}

		line.Message = matches[3]

		return line
	}

	// Format 2: level=info msg="message"
	re2 := regexp.MustCompile(`level=(\w+)\s+msg="([^"]*)"`)
	if matches := re2.FindStringSubmatch(stripped); len(matches) == 3 {
		line.Level = strings.ToUpper(matches[1])
		line.Message = matches[2]

		return line
	}

	// Extract level from anywhere in line
	if strings.Contains(strings.ToUpper(stripped), "ERROR") {
		line.Level = "ERROR"
	} else if strings.Contains(strings.ToUpper(stripped), "WARN") {
		line.Level = "WARN"
	} else if strings.Contains(strings.ToUpper(stripped), "DEBUG") {
		line.Level = "DEBUG"
	}

	return line
}
