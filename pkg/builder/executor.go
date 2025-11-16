package builder

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ethpandaops/xcli/pkg/ui"
)

// Executor runs builds in parallel respecting dependencies.
type Executor struct {
	graph   *BuildGraph
	verbose bool
}

// NewExecutor creates a new build executor for a graph.
func NewExecutor(graph *BuildGraph, verbose bool) *Executor {
	return &Executor{
		graph:   graph,
		verbose: verbose,
	}
}

// Execute runs all builds in the graph with maximum parallelization.
// Returns error if any build fails.
func (e *Executor) Execute(ctx context.Context) error {
	// Start with root nodes (no dependencies)
	roots := e.graph.GetRoots()

	if len(roots) == 0 {
		return fmt.Errorf("no root nodes found in build graph")
	}

	// Create progress bar if not in verbose mode
	var progressBar *ui.ProgressBar
	if !e.verbose {
		progressBar = ui.NewProgressBar("Building repositories", len(e.graph.nodes))
		// Note: Don't defer Stop() - progress bar auto-completes at 100%
		// Calling Stop() again causes duplicate rendering
	}

	var wg sync.WaitGroup

	errChan := make(chan error, len(e.graph.nodes))
	started := make(map[string]bool, len(e.graph.nodes)) // Track which nodes have been started

	var startMu sync.Mutex

	// Helper to start a node if not already started (declared as var for self-reference)
	var startNode func(*BuildNode)

	startNode = func(n *BuildNode) {
		startMu.Lock()

		if started[n.Name] {
			startMu.Unlock()

			return
		}

		started[n.Name] = true

		startMu.Unlock()

		wg.Add(1)

		go func(node *BuildNode) {
			defer wg.Done()

			if err := e.executeNode(ctx, node, errChan, startNode, progressBar); err != nil {
				errChan <- err
			}
		}(n)
	}

	// Kick off root nodes
	for _, node := range roots {
		startNode(node)
	}

	// Wait for all builds to complete
	wg.Wait()
	close(errChan)

	// Collect errors
	errors := make([]error, 0, len(e.graph.nodes))
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		// Stop progress bar on error
		if progressBar != nil {
			_ = progressBar.Stop()
		}

		return fmt.Errorf("build failed with %d errors: %v", len(errors), errors)
	}

	// Success case: explicitly stop progress bar to prevent duplicate rendering
	if progressBar != nil {
		_ = progressBar.Stop()
	}

	return nil
}

// executeNode builds a node after waiting for dependencies.
// Spawns goroutines for any nodes that depend on this one.
func (e *Executor) executeNode(ctx context.Context, node *BuildNode, errChan chan<- error, startNode func(*BuildNode), progressBar *ui.ProgressBar) error {
	// Wait for dependencies using channels (no busy waiting)
	for _, dep := range node.Dependencies {
		if err := dep.Wait(ctx); err != nil {
			return fmt.Errorf("dependency %s failed for %s: %w", dep.Name, node.Name, err)
		}
	}

	// Check if already built (another goroutine may have done it)
	if node.IsCompleted() {
		if e.verbose {
			fmt.Printf("⊘ Skipping %s (already built)\n", node.Name)
		}

		return nil
	}

	// Create spinner for this build if not in verbose mode
	var spinner *ui.Spinner
	if !e.verbose {
		// Use silent spinner since progress bar shows overall completion
		spinner = ui.NewSilentSpinner(fmt.Sprintf("Building %s", node.Name))
	} else {
		fmt.Printf("→ Building %s...\n", node.Name)
	}

	startTime := time.Now()

	err := node.BuildFunc()

	duration := time.Since(startTime)

	// Mark as completed and signal waiters
	node.MarkCompleted()

	// Update spinner/output based on result
	if err != nil {
		if !e.verbose {
			spinner.Fail(fmt.Sprintf("Failed to build %s", node.Name))
		}

		return fmt.Errorf("build %s failed: %w", node.Name, err)
	}

	if !e.verbose {
		// When there's a progress bar, just stop the spinner silently
		// The progress bar shows the overall completion status
		if progressBar != nil {
			_ = spinner.Stop()

			progressBar.Increment()
		} else {
			// No progress bar, show individual success message
			spinner.SuccessWithDuration(node.Name, duration)
		}
	} else {
		fmt.Printf("✓ Built %s (%.2fs)\n", node.Name, duration.Seconds())
	}

	// Start any nodes that depend on this one
	dependents := e.graph.GetDependents(node.Name)
	for _, dependent := range dependents {
		startNode(dependent)
	}

	return nil
}
