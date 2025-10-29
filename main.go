package main

import (
    "log"

    "termtrack/ui/footer"
    "termtrack/ui/header" // <-- 1. Import the new header package
    mapview "termtrack/ui/map"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
)

// mapShapePath is the path to your downloaded shapefile
const mapShapePath = "mapdata/ne_10m_admin_1_states_provinces.shp"

// model holds the application's state
type model struct {
    width  int // Terminal width
    height int // Terminal height

    headerModel header.Model // <-- 2. Add header to the main model
    mapModel    mapview.Model // The map component
    footerModel footer.Model  // The footer component

    err error // Store any errors
}

// initialModel creates the starting model
func initialModel() model {
    // Create the map model
    mapMod, err := mapview.New(mapShapePath)
    if err != nil {
        return model{err: err} // Store the loading error
    }

    // Create the footer model
    footerMod := footer.New(mapShapePath)

    // Create the header model
    headerMod := header.New() // <-- 3. Initialize the header

    // Set the initial zoom on the footer
    footerMod.SetZoom(mapMod.GetZoomLevel())

    return model{
        headerModel: headerMod, // <-- 4. Add header to the returned model
        mapModel:    mapMod,
        footerModel: footerMod,
    }
}

func (m model) Init() tea.Cmd {
    return nil // No initial commands
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    // --- Global Error Handling ---
    if m.err != nil {
        if _, ok := msg.(tea.KeyMsg); ok {
            return m, tea.Quit // Quit on any key if there's an error
        }
        return m, nil
    }

    var (
        headerCmd tea.Cmd // <-- 5. Add a command for the header
        mapCmd    tea.Cmd
        footerCmd tea.Cmd
        cmds      []tea.Cmd
    )

    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        m.width = msg.Width
        m.height = msg.Height

        // --- Layout ---
        // 6. Adjust layout for all three components
        headerHeight := 1
        footerHeight := 1
        mapHeight := m.height - headerHeight - footerHeight

        // Send resized messages to children
        headerMsg := tea.WindowSizeMsg{Width: m.width, Height: headerHeight}
        m.headerModel, headerCmd = m.headerModel.Update(headerMsg)

        mapMsg := tea.WindowSizeMsg{Width: m.width, Height: mapHeight}
        m.mapModel, mapCmd = m.mapModel.Update(mapMsg)

        footerMsg := tea.WindowSizeMsg{Width: m.width, Height: footerHeight}
        m.footerModel, footerCmd = m.footerModel.Update(footerMsg)

        cmds = append(cmds, headerCmd, mapCmd, footerCmd)

    case tea.KeyMsg:
        switch msg.String() {
        case "q", "ctrl+c", "esc":
            return m, tea.Quit
        default:
            // Pass all other keys to the map model
            m.mapModel, mapCmd = m.mapModel.Update(msg)
            cmds = append(cmds, mapCmd)

            // Sync footer zoom level after map update
            m.footerModel.SetZoom(m.mapModel.GetZoomLevel())
        }

    default:
        // Pass any other messages to all children
        m.headerModel, headerCmd = m.headerModel.Update(msg) // <-- 7. Pass messages to header
        m.mapModel, mapCmd = m.mapModel.Update(msg)
        m.footerModel, footerCmd = m.footerModel.Update(msg)
        cmds = append(cmds, headerCmd, mapCmd, footerCmd)
    }

    return m, tea.Batch(cmds...)
}

func (m model) View() string {
    // --- Error View ---
    if m.err != nil {
        errorStyle := lipgloss.NewStyle().
            Width(m.width).
            Height(m.height).
            Border(lipgloss.DoubleBorder(), true).
            BorderForeground(lipgloss.Color("9")).
            Padding(1).
            Align(lipgloss.Center, lipgloss.Center)
        return errorStyle.Render(
            "Error loading map:\n\n" + m.err.Error() +
                "\n\nPress any key to quit.",
        )
    }

    // --- Normal View ---
    headerView := m.headerModel.View() // <-- 8. Get the header's view
    mapView := m.mapModel.View()
    footerView := m.footerModel.View()

    // Stack them vertically
    // 9. Add the header to the vertical join
    return lipgloss.JoinVertical(lipgloss.Left,
        headerView,
        mapView,
        footerView,
    )
}

func main() {
    p := tea.NewProgram(initialModel(), tea.WithAltScreen())
    if _, err := p.Run(); err != nil {
        log.Fatalf("Alas, there's been an error: %v", err)
    }
}