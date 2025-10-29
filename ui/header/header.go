package header

import (
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
)

// Model holds the header's state
type Model struct {
    width int
    style lipgloss.Style
}

// New creates a new header model
func New() Model {
    return Model{
        width: 80, // default
        style: lipgloss.NewStyle().
            Padding(0, 1). // Left/Right padding
            Background(lipgloss.Color("63")). // A nice blue
            Foreground(lipgloss.Color("255")), // White text
    }
}

func (m Model) Init() tea.Cmd {
    return nil
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        m.width = msg.Width // Store the width
    }
    return m, nil
}

func (m Model) View() string {
    // Render the title, forcing it to fill the width
    return m.style.Width(m.width).Render("TermTrack")
}