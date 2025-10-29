package footer

import (
    "fmt"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
)

// Model holds the footer's state
type Model struct {
    width        int
    mapShapePath string
    zoomLevel    float64
}

// New creates a new footer model
func New(mapShapePath string) Model {
    return Model{
        width:        80, // Default
        mapShapePath: mapShapePath,
        zoomLevel:    1.0,
    }
}

func (m Model) Init() tea.Cmd {
    return nil
}

// SetZoom allows the parent model to update the zoom level
func (m *Model) SetZoom(z float64) {
    m.zoomLevel = z
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        m.width = msg.Width // Just store the width
    }
    return m, nil
}

func (m Model) View() string {
    footerStyle := lipgloss.NewStyle().
        Foreground(lipgloss.Color("240")).
        Padding(0, 1)

    // Calculate zoom level
    footerLeft := footerStyle.Render(fmt.Sprintf(
        "TermTrack | Map: %s | Zoom: %.1fx",
        m.mapShapePath, m.zoomLevel,
    ))

    footerHelp := "Pan: j/k/l/; | Zoom: K/L | Reset: r | Quit: q"

    // Use the component's width
    footerRight := footerStyle.Width(m.width - lipgloss.Width(footerLeft) - 1).
        Align(lipgloss.Right).
        Render(footerHelp)

    return lipgloss.JoinHorizontal(lipgloss.Left, footerLeft, footerRight)
}