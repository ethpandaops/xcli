package tui

import (
	"sync"
	"time"
)

// EventType represents different event types in the TUI system.
type EventType string

const (
	// EventServiceStarted is emitted when a service starts.
	EventServiceStarted EventType = "service.started"
	// EventServiceStopped is emitted when a service stops.
	EventServiceStopped EventType = "service.stopped"
	// EventServiceCrashed is emitted when a service crashes.
	EventServiceCrashed EventType = "service.crashed"
	// EventHealthChanged is emitted when service health status changes.
	EventHealthChanged EventType = "health.changed"
	// EventLogLine is emitted when a new log line is received.
	EventLogLine EventType = "log.line"
)

// Event represents a system event that can be published through the event bus.
type Event struct {
	Type      EventType
	Service   string
	Timestamp time.Time
	Data      any
}

// EventBus handles publishing and subscription of events for reactive TUI updates.
// It provides a thread-safe pub/sub mechanism for decoupling event producers
// from consumers.
type EventBus struct {
	subscribers []chan Event
	mu          sync.RWMutex
}

// NewEventBus creates a new event bus instance.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make([]chan Event, 0),
	}
}

// Subscribe returns a channel that receives all events published to the bus.
// Each subscriber gets a buffered channel with capacity of 100 events.
func (eb *EventBus) Subscribe() chan Event {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	ch := make(chan Event, 100)
	eb.subscribers = append(eb.subscribers, ch)

	return ch
}

// Publish sends an event to all subscribers. Events are delivered best-effort;
// if a subscriber's channel is full, the event is skipped for that subscriber
// rather than blocking the publisher.
func (eb *EventBus) Publish(event Event) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	for _, ch := range eb.subscribers {
		select {
		case ch <- event:
		default:
			// Skip if channel is full to avoid blocking the publisher
		}
	}
}

// Close closes all subscriber channels. After Close, the event bus
// should not be used further.
func (eb *EventBus) Close() {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	for _, ch := range eb.subscribers {
		close(ch)
	}

	eb.subscribers = nil
}
