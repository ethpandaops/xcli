package diagnostic

import (
	"regexp"
	"strings"
)

// ErrorPattern defines a known error pattern and its corresponding hint.
type ErrorPattern struct {
	// Name is a unique identifier for this pattern.
	Name string
	// Pattern is a compiled regex to match against build output.
	Pattern *regexp.Regexp
	// Contains provides an alternative to regex - check if output contains these strings.
	// All strings must be present for the pattern to match.
	Contains []string
	// Service optionally restricts this pattern to match only a specific service.
	Service string
	// Phase optionally restricts this pattern to match only a specific build phase.
	Phase BuildPhase
	// Hint provides a clear explanation of what went wrong.
	Hint string
	// Suggestion provides specific commands or actions to fix the issue.
	Suggestion string
	// Confidence indicates how confident we are that this pattern correctly identifies the issue.
	Confidence string
}

// Diagnosis contains the result of pattern matching against a build result.
type Diagnosis struct {
	// PatternName is the name of the matched pattern.
	PatternName string
	// Matched indicates whether any pattern matched.
	Matched bool
	// Hint provides a clear explanation of what went wrong.
	Hint string
	// Suggestion provides specific commands or actions to fix the issue.
	Suggestion string
	// Confidence indicates how confident we are in this diagnosis ("high", "medium", "low").
	Confidence string
}

// PatternMatcher matches errors against known patterns to provide diagnostics.
type PatternMatcher struct {
	patterns []ErrorPattern
}

// NewPatternMatcher creates a new matcher with built-in patterns.
func NewPatternMatcher() *PatternMatcher {
	m := &PatternMatcher{
		patterns: make([]ErrorPattern, 0, 32),
	}
	m.registerPatterns()

	return m
}

// Match finds the best matching pattern for a build result.
// Returns nil if no pattern matches.
func (m *PatternMatcher) Match(result *BuildResult) *Diagnosis {
	if result == nil {
		return nil
	}

	// Combine stderr and stdout for matching
	output := result.Stderr + "\n" + result.Stdout
	if strings.TrimSpace(output) == "" {
		return nil
	}

	// Normalize output for matching
	lowerOutput := strings.ToLower(output)

	var bestMatch *ErrorPattern

	var bestScore int

	for i := range m.patterns {
		pattern := &m.patterns[i]

		// Check service filter
		if pattern.Service != "" && pattern.Service != result.Service {
			continue
		}

		// Check phase filter
		if pattern.Phase != "" && pattern.Phase != result.Phase {
			continue
		}

		// Calculate match score
		score := m.calculateMatchScore(pattern, lowerOutput, output)
		if score > bestScore {
			bestScore = score
			bestMatch = pattern
		}
	}

	if bestMatch == nil {
		return nil
	}

	return &Diagnosis{
		PatternName: bestMatch.Name,
		Matched:     true,
		Hint:        bestMatch.Hint,
		Suggestion:  bestMatch.Suggestion,
		Confidence:  bestMatch.Confidence,
	}
}

// AddPattern adds a custom pattern to the matcher.
func (m *PatternMatcher) AddPattern(pattern ErrorPattern) {
	m.patterns = append(m.patterns, pattern)
}

// MatchAll returns all patterns that match a build result, sorted by confidence.
func (m *PatternMatcher) MatchAll(result *BuildResult) []*Diagnosis {
	if result == nil {
		return nil
	}

	// Combine stderr and stdout for matching
	output := result.Stderr + "\n" + result.Stdout
	if strings.TrimSpace(output) == "" {
		return nil
	}

	lowerOutput := strings.ToLower(output)
	diagnoses := make([]*Diagnosis, 0, 4)

	for i := range m.patterns {
		pattern := &m.patterns[i]

		// Check service filter
		if pattern.Service != "" && pattern.Service != result.Service {
			continue
		}

		// Check phase filter
		if pattern.Phase != "" && pattern.Phase != result.Phase {
			continue
		}

		// Check if pattern matches
		if m.calculateMatchScore(pattern, lowerOutput, output) > 0 {
			diagnoses = append(diagnoses, &Diagnosis{
				PatternName: pattern.Name,
				Matched:     true,
				Hint:        pattern.Hint,
				Suggestion:  pattern.Suggestion,
				Confidence:  pattern.Confidence,
			})
		}
	}

	// Sort by confidence
	sortDiagnosesByConfidence(diagnoses)

	return diagnoses
}

// DiagnoseOutput is a convenience function that creates a PatternMatcher and matches output.
func DiagnoseOutput(service string, phase BuildPhase, stderr, stdout string) *Diagnosis {
	matcher := NewPatternMatcher()

	return matcher.Match(&BuildResult{
		Service: service,
		Phase:   phase,
		Stderr:  stderr,
		Stdout:  stdout,
	})
}

// calculateMatchScore determines how well a pattern matches the output.
// Returns 0 for no match, higher scores for better matches.
func (m *PatternMatcher) calculateMatchScore(pattern *ErrorPattern, lowerOutput, originalOutput string) int {
	score := 0

	// Check regex pattern
	if pattern.Pattern != nil {
		if pattern.Pattern.MatchString(originalOutput) {
			score += 10
		} else if pattern.Pattern.MatchString(lowerOutput) {
			score += 8
		} else {
			return 0 // Regex must match if defined
		}
	}

	// Check contains strings
	if len(pattern.Contains) > 0 {
		allFound := true

		for _, substr := range pattern.Contains {
			if !strings.Contains(lowerOutput, strings.ToLower(substr)) {
				allFound = false

				break
			}
		}

		if !allFound {
			if pattern.Pattern == nil {
				return 0 // Contains must match if no regex is defined
			}
		} else {
			score += len(pattern.Contains) * 2
		}
	}

	// Bonus for confidence level
	switch pattern.Confidence {
	case "high":
		score += 5
	case "medium":
		score += 3
	case "low":
		score += 1
	}

	return score
}

// registerPatterns adds all built-in patterns.
func (m *PatternMatcher) registerPatterns() {
	// Proto generation errors
	m.registerProtoPatterns()

	// Go build errors
	m.registerGoBuildPatterns()

	// TypeScript/pnpm errors
	m.registerPnpmPatterns()

	// Make errors
	m.registerMakePatterns()

	// xcli specific errors
	m.registerXcliPatterns()

	// Docker errors
	m.registerDockerPatterns()
}

// registerProtoPatterns adds patterns for protobuf generation errors.
func (m *PatternMatcher) registerProtoPatterns() {
	m.patterns = append(m.patterns,
		ErrorPattern{
			Name:    "proto-undefined-type",
			Pattern: regexp.MustCompile(`(?i)undefined:\s*\w+|not declared`),
			Phase:   PhaseProtoGen,
			Hint:    "A type or identifier is undefined in the protobuf generation. This usually means a type is referenced before it's defined or an import is missing.",
			Suggestion: `Check your proto files for:
  1. Missing imports: import "path/to/dependency.proto";
  2. Typos in type names
  3. Circular dependencies between proto files

Run: make proto -C <repo> with verbose output to see which file has the error`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:    "proto-type-mismatch",
			Pattern: regexp.MustCompile(`(?i)type mismatch|cannot convert|incompatible type`),
			Phase:   PhaseProtoGen,
			Hint:    "There is a type incompatibility in the protobuf definitions. A field is using a type that doesn't match its definition.",
			Suggestion: `Check your proto files for:
  1. Mismatched field types (e.g., int32 vs int64)
  2. Changed message types without updating references
  3. Enum values used where messages are expected

Ensure all proto files are regenerated together: xcli lab rebuild xatu-cbt`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:    "proto-import-not-found",
			Pattern: regexp.MustCompile(`import\s+["'][^"']+["']\s+was not found|could not find import`),
			Phase:   PhaseProtoGen,
			Hint:    "A proto import path could not be resolved. The referenced proto file doesn't exist at the expected location.",
			Suggestion: `Fix the import path:
  1. Verify the imported .proto file exists
  2. Check include paths in your protoc command
  3. Ensure google/protobuf imports are available

You may need to run: make proto-deps or install protobuf dependencies`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:    "proto-syntax-error",
			Pattern: regexp.MustCompile(`(?i)syntax error|unexpected token|expected\s+['"][^'"]+['"]\s+but found`),
			Phase:   PhaseProtoGen,
			Hint:    "There is a syntax error in one of the proto files. This could be a missing semicolon, brace, or invalid keyword.",
			Suggestion: `Check the proto file mentioned in the error for:
  1. Missing semicolons after field definitions
  2. Unclosed braces or parentheses
  3. Invalid field numbers or reserved keywords

Run protoc with --error_format=text for clearer error messages`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:     "protoc-not-found",
			Contains: []string{"protoc", "not found"},
			Phase:    PhaseProtoGen,
			Hint:     "The protoc compiler is not installed or not in PATH.",
			Suggestion: `Install protoc:
  macOS: brew install protobuf
  Linux: apt install protobuf-compiler

Then verify: protoc --version`,
			Confidence: "high",
		},
	)
}

// registerGoBuildPatterns adds patterns for Go build errors.
func (m *PatternMatcher) registerGoBuildPatterns() {
	m.patterns = append(m.patterns,
		ErrorPattern{
			Name:    "go-undefined-identifier",
			Pattern: regexp.MustCompile(`undefined:\s*\w+`),
			Phase:   PhaseBuild,
			Hint:    "A Go identifier (function, variable, or type) is not defined. This usually means a missing import, typo, or unexported name.",
			Suggestion: `Check for:
  1. Missing import statement
  2. Typo in the identifier name
  3. Using unexported (lowercase) name from another package
  4. Stale generated code that needs regeneration

Run: go build -v to see detailed compilation info`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:    "go-missing-package",
			Pattern: regexp.MustCompile(`cannot find package|no required module provides package`),
			Phase:   PhaseBuild,
			Hint:    "A required Go package is not available. The dependency might be missing from go.mod or not downloaded.",
			Suggestion: `Fix missing package:
  1. go mod tidy - clean up and add missing deps
  2. go get <package> - explicitly add the package
  3. go mod download - download all dependencies

If using a local replace directive, verify the path exists`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:    "go-import-cycle",
			Pattern: regexp.MustCompile(`import cycle not allowed|package.*imports.*imports`),
			Phase:   PhaseBuild,
			Hint:    "There is a circular import between packages. Package A imports B which imports A (directly or indirectly).",
			Suggestion: `Break the import cycle by:
  1. Moving shared types to a separate package
  2. Using interfaces to decouple packages
  3. Restructuring code to have clear dependency direction

Use: go list -deps ./... | sort | uniq -d to find cycles`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:     "go-too-many-errors",
			Contains: []string{"too many errors"},
			Phase:    PhaseBuild,
			Hint:     "The Go compiler stopped after encountering too many errors. Fix the first few errors first, as later errors are often cascading from earlier ones.",
			Suggestion: `Focus on the first error in the output:
  1. Scroll up to find the first error message
  2. Fix it and rebuild
  3. Many subsequent errors will likely disappear

Often caused by missing import or undefined type that's used everywhere`,
			Confidence: "medium",
		},
		ErrorPattern{
			Name:    "go-type-mismatch",
			Pattern: regexp.MustCompile(`cannot use .* as .* in|incompatible types|cannot convert`),
			Phase:   PhaseBuild,
			Hint:    "A type mismatch occurred. A value of one type is being used where another type is expected.",
			Suggestion: `Check the error location for:
  1. Wrong type being passed to function
  2. Assignment to incompatible variable
  3. Interface implementation mismatch

If this involves generated code, regenerate protos: xcli lab rebuild xatu-cbt`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:    "go-not-enough-arguments",
			Pattern: regexp.MustCompile(`not enough arguments|too few arguments|missing argument`),
			Phase:   PhaseBuild,
			Hint:    "A function is being called with fewer arguments than required.",
			Suggestion: `Check the function signature has changed:
  1. Look at the function definition
  2. Update all call sites with required arguments
  3. If using generated code, regenerate it first

Run: go doc <package>.<Function> to see current signature`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:    "go-too-many-arguments",
			Pattern: regexp.MustCompile(`too many arguments`),
			Phase:   PhaseBuild,
			Hint:    "A function is being called with more arguments than it accepts.",
			Suggestion: `Check the function signature:
  1. The function may have been updated with fewer parameters
  2. Remove extra arguments from call sites
  3. If using generated code, regenerate it first`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:    "go-no-new-variables",
			Pattern: regexp.MustCompile(`no new variables on left side of :=`),
			Phase:   PhaseBuild,
			Hint:    "Using := when all variables on the left are already declared. Use = instead.",
			Suggestion: `Change := to = when reassigning existing variables:

  // Wrong:
  x := 1
  x := 2  // error

  // Correct:
  x := 1
  x = 2`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:    "go-declared-not-used",
			Pattern: regexp.MustCompile(`declared (and|but) not used`),
			Phase:   PhaseBuild,
			Hint:    "A variable is declared but never used. Go doesn't allow unused variables.",
			Suggestion: `Either:
  1. Use the variable where intended
  2. Remove the declaration
  3. Use _ to explicitly ignore: _ = unusedVar`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:    "go-imported-not-used",
			Pattern: regexp.MustCompile(`imported and not used`),
			Phase:   PhaseBuild,
			Hint:    "A package is imported but not used. Go doesn't allow unused imports.",
			Suggestion: `Either:
  1. Use something from the package
  2. Remove the import
  3. Use blank import if needed for side effects: import _ "package"

Run: goimports -w . to auto-fix imports`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:    "go-mod-tidy-needed",
			Pattern: regexp.MustCompile(`go\.sum contains unexpected module|missing go\.sum entry`),
			Phase:   PhaseBuild,
			Hint:    "The go.sum file is out of sync with go.mod. Dependencies need to be reconciled.",
			Suggestion: `Run:
  go mod tidy

This will update both go.mod and go.sum to match your imports`,
			Confidence: "high",
		},
	)
}

// registerPnpmPatterns adds patterns for TypeScript/pnpm errors.
func (m *PatternMatcher) registerPnpmPatterns() {
	m.patterns = append(m.patterns,
		ErrorPattern{
			Name:    "ts-cannot-find-module",
			Pattern: regexp.MustCompile(`Cannot find module|Module not found`),
			Phase:   PhaseFrontendGen,
			Hint:    "TypeScript cannot find a required module. The npm package may not be installed or the import path is wrong.",
			Suggestion: `Fix missing module:
  1. pnpm install - install all dependencies
  2. pnpm add <package> - add specific package
  3. Check import path is correct

For generated types: xcli lab rebuild lab-frontend`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:    "ts-type-error-2304",
			Pattern: regexp.MustCompile(`TS2304|Cannot find name`),
			Phase:   PhaseFrontendGen,
			Hint:    "TypeScript error TS2304: Cannot find name. A type or variable is referenced but not defined or imported.",
			Suggestion: `Check for:
  1. Missing import statement
  2. Missing type definition (@types/package)
  3. Generated types need regeneration

For API types: xcli lab rebuild lab-frontend`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:    "ts-type-error-2305",
			Pattern: regexp.MustCompile(`TS2305|has no exported member`),
			Phase:   PhaseFrontendGen,
			Hint:    "TypeScript error TS2305: Module has no exported member. The import exists but doesn't export the requested name.",
			Suggestion: `Check the module for:
  1. Renamed export
  2. Removed export
  3. Default vs named export mismatch

If importing from generated code, regenerate: xcli lab rebuild lab-frontend`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:    "pnpm-enoent",
			Pattern: regexp.MustCompile(`ENOENT|no such file or directory`),
			Phase:   PhaseFrontendGen,
			Hint:    "A file or directory was not found during pnpm operation.",
			Suggestion: `Check:
  1. Run pnpm install to ensure all dependencies are installed
  2. Verify the project directory exists
  3. Check if a build step needs to run first

For missing generated files: xcli lab rebuild lab-frontend`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:    "pnpm-specific-error",
			Pattern: regexp.MustCompile(`ERR_PNPM_[A-Z_]+`),
			Phase:   PhaseFrontendGen,
			Hint:    "A pnpm-specific error occurred. This is usually related to the package manager itself.",
			Suggestion: `Try these steps:
  1. pnpm store prune - clean the pnpm store
  2. rm -rf node_modules && pnpm install - fresh install
  3. Check pnpm version: pnpm --version

See pnpm documentation for the specific error code`,
			Confidence: "medium",
		},
		ErrorPattern{
			Name:     "pnpm-peer-deps",
			Contains: []string{"peer", "dependency", "missing"},
			Phase:    PhaseFrontendGen,
			Hint:     "A peer dependency is missing. The package requires another package to be installed alongside it.",
			Suggestion: `Fix peer dependencies:
  1. pnpm install - should auto-install peers
  2. Manually install: pnpm add <peer-package>
  3. Check package.json for version compatibility`,
			Confidence: "medium",
		},
		ErrorPattern{
			Name:    "ts-strict-null-checks",
			Pattern: regexp.MustCompile(`TS2531|TS2532|Object is possibly.*null|Object is possibly.*undefined`),
			Phase:   PhaseFrontendGen,
			Hint:    "TypeScript strict null checks caught a potential null/undefined access.",
			Suggestion: `Add null checks:
  1. Use optional chaining: obj?.property
  2. Add null check: if (obj !== null)
  3. Use non-null assertion if certain: obj!.property`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:    "vite-build-error",
			Pattern: regexp.MustCompile(`error during build|Build failed|vite:build`),
			Phase:   PhaseFrontendGen,
			Hint:    "The Vite build process failed. Check the error details above for the specific issue.",
			Suggestion: `Debug Vite build:
  1. Run pnpm build --debug for more details
  2. Check vite.config.ts for issues
  3. Verify all imports are correct

For HMR issues, try: rm -rf .vite && pnpm dev`,
			Confidence: "medium",
		},
	)
}

// registerMakePatterns adds patterns for Make errors.
func (m *PatternMatcher) registerMakePatterns() {
	m.patterns = append(m.patterns,
		ErrorPattern{
			Name:    "make-no-rule",
			Pattern: regexp.MustCompile(`No rule to make target|missing target`),
			Hint:    "Make cannot find a rule for the specified target. The target doesn't exist in the Makefile.",
			Suggestion: `Check:
  1. Target name is spelled correctly
  2. Target exists in Makefile: grep -n "^target:" Makefile
  3. You're in the correct directory

List available targets: make help (if available)`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:    "make-recipe-failed",
			Pattern: regexp.MustCompile(`recipe for target .* failed|Error \d+|make.*Error`),
			Hint:    "A command in the Makefile recipe failed. Check the output above for the specific error.",
			Suggestion: `Debug steps:
  1. Look at the command that failed above this error
  2. Run the failing command manually with verbose output
  3. Check prerequisites are met

Run with verbose: make <target> V=1`,
			Confidence: "medium",
		},
		ErrorPattern{
			Name:     "make-missing-separator",
			Contains: []string{"missing separator", "stop"},
			Hint:     "Makefile syntax error: commands must be indented with a tab, not spaces.",
			Suggestion: `Fix Makefile indentation:
  1. Use actual TAB character for recipe lines
  2. Don't mix tabs and spaces

Check with: cat -A Makefile | grep "^ " (should show ^I for tabs)`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:    "make-circular-dependency",
			Pattern: regexp.MustCompile(`Circular .* dependency dropped|recursive variable`),
			Hint:    "The Makefile has circular dependencies between targets.",
			Suggestion: `Check Makefile for circular dependencies:
  1. Target A depends on B, and B depends on A
  2. Restructure dependencies to be acyclic
  3. Use .PHONY for non-file targets`,
			Confidence: "high",
		},
	)
}

// registerXcliPatterns adds patterns for xcli-specific errors.
func (m *PatternMatcher) registerXcliPatterns() {
	m.patterns = append(m.patterns,
		ErrorPattern{
			Name:     "clickhouse-not-ready",
			Contains: []string{"connection refused", "8123"},
			Phase:    PhaseRestart,
			Hint:     "Cannot connect to ClickHouse on port 8123. The ClickHouse service is not running or not ready.",
			Suggestion: `Start ClickHouse:
  xcli lab up

Or wait for it to be ready:
  docker logs xcli-clickhouse --follow

Check status: xcli lab status`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:     "redis-not-ready",
			Contains: []string{"connection refused", "6379"},
			Phase:    PhaseRestart,
			Hint:     "Cannot connect to Redis on port 6379. The Redis service is not running or not ready.",
			Suggestion: `Start Redis:
  xcli lab up

Check status: xcli lab status
View logs: docker logs xcli-redis`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:    "port-already-in-use",
			Pattern: regexp.MustCompile(`address already in use|bind.*address already in use|EADDRINUSE`),
			Phase:   PhaseRestart,
			Hint:    "A required port is already in use by another process.",
			Suggestion: `Find and stop the conflicting process:
  lsof -i :<port> | grep LISTEN
  kill <PID>

Or change the port in your config:
  xcli lab config

Then restart: xcli lab restart`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:    "cbt-api-not-ready",
			Pattern: regexp.MustCompile(`cbt-api.*connection refused|openapi\.yaml.*ECONNREFUSED`),
			Phase:   PhaseRestart,
			Hint:    "Cannot connect to cbt-api. The service is not running or hasn't finished starting.",
			Suggestion: `Wait for cbt-api to start:
  xcli lab status

Or check logs:
  xcli lab logs cbt-api-<network>

Start services if not running: xcli lab up`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:     "config-not-found",
			Contains: []string{"config", "not found", ".xcli"},
			Hint:     "xcli configuration file not found. The lab may not be initialized.",
			Suggestion: `Initialize xcli:
  xcli init

Or check you're in the correct directory with .xcli.yaml`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:     "repo-not-found",
			Contains: []string{"repository", "not found"},
			Hint:     "A required repository is not found at the configured path.",
			Suggestion: `Check repository paths in config:
  xcli lab config

Clone missing repos:
  xcli init

Or run prerequisites: xcli lab check`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:    "permission-denied",
			Pattern: regexp.MustCompile(`permission denied|EACCES|Operation not permitted`),
			Hint:    "Permission denied when accessing a file or resource.",
			Suggestion: `Check permissions:
  1. File ownership: ls -la <file>
  2. Binary execution: chmod +x <binary>
  3. Docker permissions: ensure user is in docker group

For Docker: sudo usermod -aG docker $USER && newgrp docker`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:    "network-timeout",
			Pattern: regexp.MustCompile(`timeout|ETIMEDOUT|context deadline exceeded`),
			Phase:   PhaseRestart,
			Hint:    "A network operation timed out. The service may be slow to respond or unreachable.",
			Suggestion: `Check service health:
  xcli lab status

Increase timeout or check:
  1. Service is running: docker ps
  2. Network connectivity: ping localhost
  3. Resource usage: docker stats`,
			Confidence: "medium",
		},
		ErrorPattern{
			Name:    "database-connection-error",
			Pattern: regexp.MustCompile(`database.*connection|DB_.*refused|sql:.*connection`),
			Phase:   PhaseRestart,
			Hint:    "Cannot connect to the database. The database service may not be running.",
			Suggestion: `Check database services:
  xcli lab status

Ensure ClickHouse is running:
  docker logs xcli-clickhouse

Start services: xcli lab up`,
			Confidence: "high",
		},
	)
}

// registerDockerPatterns adds patterns for Docker-related errors.
func (m *PatternMatcher) registerDockerPatterns() {
	m.patterns = append(m.patterns,
		ErrorPattern{
			Name:     "docker-daemon-not-running",
			Contains: []string{"docker", "daemon", "not running"},
			Hint:     "Docker daemon is not running. Docker Desktop or the Docker service needs to be started.",
			Suggestion: `Start Docker:
  macOS: Open Docker Desktop
  Linux: sudo systemctl start docker

Verify: docker info`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:    "docker-image-not-found",
			Pattern: regexp.MustCompile(`image .* not found|pull access denied|manifest unknown`),
			Hint:    "Docker image not found. The image may not exist or you don't have access.",
			Suggestion: `Check:
  1. Image name is correct
  2. You're logged in: docker login
  3. Pull manually: docker pull <image>

For local images: docker build -t <name> .`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:    "docker-container-conflict",
			Pattern: regexp.MustCompile(`container .* already exists|name .* is already in use`),
			Hint:    "A container with the same name already exists.",
			Suggestion: `Remove the existing container:
  docker rm <container-name>

Or use a different name.

Clean up all xcli containers: xcli lab down`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:    "docker-no-space",
			Pattern: regexp.MustCompile(`no space left on device|out of disk space`),
			Hint:    "Docker has run out of disk space.",
			Suggestion: `Clean up Docker:
  docker system prune -a --volumes

Check space: docker system df

Increase Docker disk space in Docker Desktop settings`,
			Confidence: "high",
		},
		ErrorPattern{
			Name:    "docker-network-error",
			Pattern: regexp.MustCompile(`network .* not found|failed to create network`),
			Hint:    "Docker network error. The network doesn't exist or couldn't be created.",
			Suggestion: `Check networks:
  docker network ls

Create if needed:
  docker network create xcli

Clean up: xcli lab down && xcli lab up`,
			Confidence: "high",
		},
	)
}

// sortDiagnosesByConfidence sorts diagnoses with high confidence first.
func sortDiagnosesByConfidence(diagnoses []*Diagnosis) {
	confidenceOrder := map[string]int{
		"high":   3,
		"medium": 2,
		"low":    1,
	}

	// Simple bubble sort for small slices
	n := len(diagnoses)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if confidenceOrder[diagnoses[j].Confidence] < confidenceOrder[diagnoses[j+1].Confidence] {
				diagnoses[j], diagnoses[j+1] = diagnoses[j+1], diagnoses[j]
			}
		}
	}
}
