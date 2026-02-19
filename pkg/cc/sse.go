// Package cc implements the Command Center web dashboard for xcli.
package cc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// sseClient represents a connected SSE client.
type sseClient struct {
	events chan sseEvent
	done   chan struct{}
}

// sseEvent represents a named SSE event with JSON data.
type sseEvent struct {
	Name string
	Data any
}

// SSEHub manages SSE client connections and broadcasts events.
type SSEHub struct {
	log     logrus.FieldLogger
	clients map[*sseClient]struct{}
	mu      sync.RWMutex
	done    chan struct{}
}

// NewSSEHub creates a new SSE hub.
func NewSSEHub(log logrus.FieldLogger) *SSEHub {
	return &SSEHub{
		log:     log.WithField("component", "sse-hub"),
		clients: make(map[*sseClient]struct{}, 8),
		done:    make(chan struct{}),
	}
}

// Register adds a new SSE client and returns it.
func (h *SSEHub) Register() *sseClient {
	c := &sseClient{
		events: make(chan sseEvent, 64),
		done:   make(chan struct{}),
	}

	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()

	h.log.WithField("clients", len(h.clients)).Debug("SSE client connected")

	return c
}

// Unregister removes an SSE client.
func (h *SSEHub) Unregister(c *sseClient) {
	h.mu.Lock()

	if _, exists := h.clients[c]; !exists {
		h.mu.Unlock()

		return // Already removed (e.g. by Stop)
	}

	delete(h.clients, c)
	h.mu.Unlock()

	close(c.done)

	h.log.WithField("clients", len(h.clients)).Debug("SSE client disconnected")
}

// Broadcast sends an event to all connected clients (best-effort).
func (h *SSEHub) Broadcast(name string, data any) {
	evt := sseEvent{Name: name, Data: data}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for c := range h.clients {
		select {
		case c.events <- evt:
		default:
			// Drop event for slow clients
		}
	}
}

// Stop signals the hub to shut down.
func (h *SSEHub) Stop() {
	close(h.done)

	h.mu.Lock()
	defer h.mu.Unlock()

	for c := range h.clients {
		close(c.done)
		delete(h.clients, c)
	}
}

// ServeHTTP handles the SSE endpoint.
func (h *SSEHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	client := h.Register()

	defer h.Unregister(client)

	// Send initial keepalive
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	// Tick for keepalive
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-h.done:
			return
		case <-client.done:
			return
		case evt := <-client.events:
			data, err := json.Marshal(evt.Data)
			if err != nil {
				h.log.WithError(err).Error("Failed to marshal SSE event")

				continue
			}

			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Name, data)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}
