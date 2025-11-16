package builder

import (
	"context"
	"fmt"
	"sync"
)

// BuildNode represents a build target with dependencies.
type BuildNode struct {
	Name         string
	BuildFunc    func() error
	Dependencies []*BuildNode
	completed    bool
	done         chan struct{} // Closed when build completes
	mu           sync.Mutex
}

// NewBuildNode creates a new build node.
func NewBuildNode(name string, buildFunc func() error) *BuildNode {
	return &BuildNode{
		Name:      name,
		BuildFunc: buildFunc,
		done:      make(chan struct{}),
	}
}

// IsCompleted returns true if this node has been built.
func (n *BuildNode) IsCompleted() bool {
	n.mu.Lock()
	defer n.mu.Unlock()

	return n.completed
}

// MarkCompleted marks this node as built and signals waiters.
func (n *BuildNode) MarkCompleted() {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.completed {
		n.completed = true
		close(n.done) // Signal all waiters
	}
}

// Wait blocks until this node completes or context is cancelled.
func (n *BuildNode) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-n.done:
		return nil
	}
}

// BuildGraph manages the dependency graph for builds.
type BuildGraph struct {
	nodes      map[string]*BuildNode
	dependents map[string][]*BuildNode // reverse dependency map
	mu         sync.RWMutex
}

// NewBuildGraph creates a new empty build graph.
func NewBuildGraph() *BuildGraph {
	return &BuildGraph{
		nodes:      make(map[string]*BuildNode, 5), // Typical: 3-5 build targets
		dependents: make(map[string][]*BuildNode, 5),
	}
}

// AddNode adds a build target with its dependencies.
// Dependencies are specified by name and must already exist in the graph.
func (g *BuildGraph) AddNode(name string, buildFunc func() error, dependencyNames ...string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	node := NewBuildNode(name, buildFunc)

	// Resolve dependencies and build reverse map
	for _, depName := range dependencyNames {
		depNode, exists := g.nodes[depName]
		if !exists {
			return fmt.Errorf("dependency %s not found for node %s (add dependencies before dependents)", depName, name)
		}

		node.Dependencies = append(node.Dependencies, depNode)
		// Track that this node depends on depNode (reverse dependency)
		g.dependents[depName] = append(g.dependents[depName], node)
	}

	g.nodes[name] = node

	return nil
}

// GetRoots returns all nodes with no dependencies (can build immediately).
func (g *BuildGraph) GetRoots() []*BuildNode {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var roots []*BuildNode

	for _, node := range g.nodes {
		if len(node.Dependencies) == 0 {
			roots = append(roots, node)
		}
	}

	return roots
}

// GetDependents returns all nodes that depend on the given node name.
func (g *BuildGraph) GetDependents(name string) []*BuildNode {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return g.dependents[name]
}
