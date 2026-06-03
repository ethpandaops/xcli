package ui

import (
	"testing"
	"time"

	"github.com/acarl005/stripansi"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRendererSelectsPlainUnderTest(t *testing.T) {
	// The test binary is non-interactive, so NewRenderer must avoid the live
	// renderer regardless of the interrupt callback.
	r := NewRenderer(func() {})

	_, ok := r.(*PlainRenderer)
	assert.True(t, ok, "expected plain renderer in test mode, got %T", r)
}

func TestTreeModelRendersPhasesAndTasks(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := &treeModel{width: 60, now: base}

	m.Update(bannerMsg{text: "Starting Lab Stack"})
	m.Update(phaseMsg{title: "Phase 1: Building Xatu-CBT"})
	m.Update(addTaskMsg{id: 1, name: "Building xatu-cbt"})

	// Advance the clock so the completed task reports a real duration.
	m.now = base.Add(2 * time.Second)
	m.Update(taskDoneMsg{id: 1, status: taskOK, text: "Xatu-CBT built successfully"})

	view := stripansi.Strip(m.View())

	assert.Contains(t, view, "Starting Lab Stack")
	assert.Contains(t, view, "▌ Phase 1: Building Xatu-CBT")
	assert.Contains(t, view, "└─")
	assert.Contains(t, view, "✓ ")
	assert.Contains(t, view, "Xatu-CBT built successfully")
	assert.Contains(t, view, "2.0s")
}

func TestTreeModelTaskStatusGlyphs(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := &treeModel{width: 80, now: base}

	m.Update(phaseMsg{title: "Phase"})
	m.Update(addTaskMsg{id: 1, name: "ok task"})
	m.Update(addTaskMsg{id: 2, name: "warn task"})
	m.Update(addTaskMsg{id: 3, name: "fail task"})
	m.Update(taskDoneMsg{id: 1, status: taskOK})
	m.Update(taskDoneMsg{id: 2, status: taskWarn})
	m.Update(taskDoneMsg{id: 3, status: taskFail})

	view := stripansi.Strip(m.View())

	assert.Contains(t, view, "✓")
	assert.Contains(t, view, "⚠")
	assert.Contains(t, view, "✗")
	// First two of three tasks use a tee connector, the last uses an elbow.
	assert.Contains(t, view, "├─")
	assert.Contains(t, view, "└─")
}

func TestTreeModelImplicitPhaseForEarlyTask(t *testing.T) {
	// A task started before any Phase must still render (under a headerless
	// implicit phase) rather than being dropped.
	m := &treeModel{width: 40, now: time.Unix(0, 0)}
	m.Update(addTaskMsg{id: 1, name: "Testing external ClickHouse DSN"})

	view := stripansi.Strip(m.View())

	assert.Contains(t, view, "Testing external ClickHouse DSN")
}

func TestTreeModelInterruptInvokesCallbackThenQuits(t *testing.T) {
	called := 0
	m := &treeModel{onInterrupt: func() { called++ }}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	require.Equal(t, 1, called, "first ctrl+c should invoke the interrupt callback")
	assert.True(t, m.interrupted)
	assert.Nil(t, cmd, "first ctrl+c should not quit")

	assert.Contains(t, stripansi.Strip(m.View()), "shutting down gracefully")

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	assert.Equal(t, 1, called, "second ctrl+c should not re-invoke the callback")
	require.NotNil(t, cmd, "second ctrl+c should quit")

	msg := cmd()
	_, isQuit := msg.(tea.QuitMsg)
	assert.True(t, isQuit, "expected QuitMsg, got %T", msg)
}

func TestFormatTreeDuration(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"sub-second", 300 * time.Millisecond, "0.3s"},
		{"seconds", 6200 * time.Millisecond, "6.2s"},
		{"minutes", 64 * time.Second, "1m04s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, formatTreeDuration(tt.d))
		})
	}
}
