package main

import (
	"time"
	tea "github.com/charmbracelet/bubbletea"
)

// How often we't like to re-render the map
const renderFrameRate = time.Millisecond * 50 // ~20fps

// TickMsg is the message sent on every render tick
type TickMsg struct{}

// TickCmd returns a command that sends a TickMsg after our frame rate delay
func TickCmd() tea.Cmd {
	return tea.Tick(renderFrameRate, func(t time.Time) tea.Msg {
		return TickMsg{}
	})
}