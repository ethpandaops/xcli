package infrastructure

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"

	"github.com/sirupsen/logrus"
)

const (
	// Production cluster hardcoded details.
	prodK8sContext = "platform-analytics-hel1-production"
	prodNamespace  = "xatu"

	// localRedisContainer is the local Docker container name for xatu-cbt Redis.
	localRedisContainer = "xatu-cbt-redis"

	// redisBoundsKeyPrefix is the Redis key prefix for external model bounds.
	redisBoundsKeyPrefix = "cbt:external:"

	// luaDumpExternalBounds is a Lua script that fetches all external bounds keys and values
	// in a single Redis round trip. Returns alternating key/value pairs.
	luaDumpExternalBounds = `local keys = redis.call('KEYS', ARGV[1]) local result = {} for i, key in ipairs(keys) do result[#result + 1] = key result[#result + 1] = redis.call('GET', key) end return result`
)

// prodRedisDetails returns the production Redis pod name and password k8s secret name for a network.
func prodRedisDetails(network string) (podName, secretName string) {
	return fmt.Sprintf("%s-xatu-cbt-redis-node-0", network),
		fmt.Sprintf("%s-xatu-cbt-redis", network)
}

// BoundsSeeder handles seeding CBT external bounds from production Redis.
type BoundsSeeder struct {
	log logrus.FieldLogger
}

// NewBoundsSeeder creates a new bounds seeder.
func NewBoundsSeeder(log logrus.FieldLogger) *BoundsSeeder {
	return &BoundsSeeder{
		log: log.WithField("component", "bounds_seeder"),
	}
}

// redisBound represents a key-value pair from Redis.
type redisBound struct {
	Key   string
	Value string
}

// SeedFromProduction fetches external model bounds from production Redis
// and inserts them into local Redis. This avoids slow initial full scans
// of external tables on the remote ClickHouse cluster.
//
// Uses a single Lua EVAL to bulk-fetch all bounds in one round trip,
// then pipes them into local Redis via --pipe for efficient insertion.
func (s *BoundsSeeder) SeedFromProduction(ctx context.Context, network string, redisDB int) error {
	s.log.WithField("network", network).Info("Fetching external bounds from production Redis")

	// Check kubectl is available
	if err := s.checkKubectl(ctx); err != nil {
		return fmt.Errorf("kubectl not available: %w", err)
	}

	// Get production Redis password
	_, secretName := prodRedisDetails(network)

	password, err := s.getRedisPassword(ctx, secretName)
	if err != nil {
		return fmt.Errorf("getting Redis password for %s: %w", network, err)
	}

	// Bulk-fetch all external bounds in a single Lua EVAL call
	bounds, err := s.fetchAllBounds(ctx, network, password)
	if err != nil {
		return fmt.Errorf("fetching bounds for %s: %w", network, err)
	}

	if len(bounds) == 0 {
		s.log.WithField("network", network).Warn("No external bounds found in production Redis")

		return nil
	}

	// Bulk-insert into local Redis via --pipe
	if err := s.bulkInsertLocal(ctx, redisDB, bounds); err != nil {
		return fmt.Errorf("inserting bounds for %s: %w", network, err)
	}

	s.log.WithFields(logrus.Fields{
		"network": network,
		"count":   len(bounds),
	}).Info("External bounds seeded from production")

	return nil
}

// checkKubectl verifies kubectl is available.
func (s *BoundsSeeder) checkKubectl(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "kubectl", "version", "--client")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kubectl command failed: %w", err)
	}

	return nil
}

// getRedisPassword retrieves the Redis password from a k8s secret.
func (s *BoundsSeeder) getRedisPassword(ctx context.Context, secretName string) (string, error) {
	args := []string{
		"--context", prodK8sContext,
		"-n", prodNamespace,
		"get", "secret", secretName,
		"-o", "jsonpath={.data.redis-password}",
	}

	cmd := exec.CommandContext(ctx, "kubectl", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("kubectl get secret failed: %w\nOutput: %s", err, string(output))
	}

	// The password is base64 encoded in the jsonpath output
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(output)))
	if err != nil {
		return "", fmt.Errorf("decoding password: %w", err)
	}

	return strings.TrimSpace(string(decoded)), nil
}

// fetchAllBounds uses a single Lua EVAL to get all external bounds keys and values
// from production Redis in one round trip.
func (s *BoundsSeeder) fetchAllBounds(ctx context.Context, network, password string) ([]redisBound, error) {
	podName, _ := prodRedisDetails(network)

	args := []string{
		"--context", prodK8sContext,
		"-n", prodNamespace,
		"exec", podName,
		"-c", "redis",
		"--",
		"redis-cli",
		"-a", password,
		"--no-auth-warning",
		"EVAL", luaDumpExternalBounds,
		"0", // numkeys
		redisBoundsKeyPrefix + "*",
	}

	cmd := exec.CommandContext(ctx, "kubectl", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("redis EVAL failed: %w\nOutput: %s", err, string(output))
	}

	return s.parseLuaEvalOutput(string(output))
}

// parseLuaEvalOutput parses redis-cli output from Lua EVAL that returns
// alternating key/value pairs. redis-cli prints each array element on its own line.
func (s *BoundsSeeder) parseLuaEvalOutput(output string) ([]redisBound, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) == 0 || (len(lines) == 1 && strings.TrimSpace(lines[0]) == "") {
		return nil, nil
	}

	// Filter out empty lines
	filtered := make([]string, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && line != "(empty array)" && line != "(empty list or set)" {
			filtered = append(filtered, line)
		}
	}

	if len(filtered)%2 != 0 {
		return nil, fmt.Errorf("unexpected odd number of lines in EVAL output (%d)", len(filtered))
	}

	bounds := make([]redisBound, 0, len(filtered)/2)

	for i := 0; i < len(filtered); i += 2 {
		key := filtered[i]
		value := filtered[i+1]

		// redis-cli may prefix array elements with "N) " numbering
		key = stripRedisPrefix(key)
		value = stripRedisPrefix(value)

		bounds = append(bounds, redisBound{Key: key, Value: value})
	}

	return bounds, nil
}

// stripRedisPrefix removes the "N) " prefix that redis-cli adds to array elements.
func stripRedisPrefix(s string) string {
	// Look for pattern like "1) " or "42) "
	idx := strings.Index(s, ") ")
	if idx > 0 && idx < 6 {
		// Verify everything before ") " is digits
		prefix := s[:idx]
		allDigits := true

		for _, c := range prefix {
			if c < '0' || c > '9' {
				allDigits = false

				break
			}
		}

		if allDigits {
			return s[idx+2:]
		}
	}

	return s
}

// bulkInsertLocal inserts all bounds into local Redis using --pipe for efficiency.
// This sends all SET commands in a single docker exec call.
func (s *BoundsSeeder) bulkInsertLocal(ctx context.Context, redisDB int, bounds []redisBound) error {
	// Build Redis protocol for all SET commands
	var protocol bytes.Buffer

	for _, b := range bounds {
		// RESP protocol: *3\r\n$3\r\nSET\r\n${keyLen}\r\n{key}\r\n${valLen}\r\n{value}\r\n
		fmt.Fprintf(&protocol, "*3\r\n$3\r\nSET\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n",
			len(b.Key), b.Key, len(b.Value), b.Value)
	}

	args := []string{
		"exec", "-i", localRedisContainer,
		"redis-cli",
		"-n", fmt.Sprintf("%d", redisDB),
		"--pipe",
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = &protocol

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("redis --pipe failed: %w\nOutput: %s", err, string(output))
	}

	s.log.WithField("output", strings.TrimSpace(string(output))).Debug("Redis pipe output")

	return nil
}

// CheckNeedsSeeding checks if local Redis has any external bounds for the given network.
func (s *BoundsSeeder) CheckNeedsSeeding(ctx context.Context, redisDB int) (bool, error) {
	args := []string{
		"exec", localRedisContainer,
		"redis-cli",
		"-n", fmt.Sprintf("%d", redisDB),
		"KEYS", redisBoundsKeyPrefix + "*",
	}

	cmd := exec.CommandContext(ctx, "docker", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("redis KEYS failed: %w\nOutput: %s", err, string(output))
	}

	result := strings.TrimSpace(string(output))

	// Empty result or "(empty array)" means no keys exist
	return result == "" || strings.Contains(result, "empty"), nil
}
