package tui

import (
	"regexp"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Model is the Bubbletea application state.
type Model struct {
	// Config
	maxLogLines int // Maximum log lines per service (-1 for unlimited)

	// Data
	wrapper        *OrchestratorWrapper
	services       []ServiceInfo
	infrastructure []InfraInfo
	logs           map[string][]LogLine // service -> log lines
	health         map[string]HealthStatus

	// UI State
	selectedIndex    int
	activePanel      string // "services", "logs", "infra"
	logScroll        int
	followMode       bool // Auto-scroll to bottom as new logs arrive
	selectedLogIndex int  // Selected log line index (relative to visible logs)
	logDetailMode    bool // Whether showing full log detail overlay

	// Filter State
	filterMode     bool           // Whether the filter input is active
	filterInput    string         // Current filter input text
	filterActive   bool           // Whether a filter is currently applied
	filterRegex    string         // The filter regex pattern string
	filterCompiled *regexp.Regexp // Pre-compiled regex (nil if invalid)
	filterError    error          // Error from regex compilation (nil if valid)

	// Menu State
	showMenu    bool         // Whether the context menu is visible
	menuActions []MenuAction // Current menu actions

	// Activity State
	activity      string    // Current activity description (empty if idle)
	activityStart time.Time // When the activity started

	// Lifecycle
	updateTicker  *time.Ticker
	logStreamer   *LogStreamer
	healthMonitor *HealthMonitor
	eventBus      *EventBus

	// Dimensions
	width  int
	height int

	// Status
	lastUpdate time.Time
}

// NewModel creates initial model.
func NewModel(wrapper *OrchestratorWrapper, maxLogLines int) Model {
	eventBus := NewEventBus()

	return Model{
		maxLogLines:    maxLogLines,
		wrapper:        wrapper,
		services:       []ServiceInfo{},
		infrastructure: []InfraInfo{},
		logs:           make(map[string][]LogLine),
		health:         make(map[string]HealthStatus),
		selectedIndex:  0,
		activePanel:    panelServices,
		logScroll:      0,
		followMode:     true, // Start in follow mode
		filterMode:     false,
		filterInput:    "",
		filterActive:   false,
		filterRegex:    "",
		updateTicker:   time.NewTicker(2 * time.Second),
		eventBus:       eventBus,
	}
}

// Init is called when program starts.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tick(),
		waitForEvent(m.eventBus.Subscribe()),
	)
}

// Messages for Bubbletea.
type tickMsg time.Time
type eventMsg Event
type logMsg LogLine
type healthMsg map[string]HealthStatus
type activityDoneMsg struct {
	err error
}

// tick returns a command that waits for next tick.
func tick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// waitForEvent returns a command that waits for next event.
func waitForEvent(ch chan Event) tea.Cmd {
	return func() tea.Msg {
		return eventMsg(<-ch)
	}
}
