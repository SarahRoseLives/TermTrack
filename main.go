package main

import (
	"bufio"
	"log"
	"net" // <-- Import 'net'
	"time"

	"termtrack/sbs"
	"termtrack/ui/footer"
	"termtrack/ui/header"
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

	headerModel header.Model
	mapModel    mapview.Model
	footerModel footer.Model

	// --- SBS State ---
	sbsScanner *bufio.Scanner
	sbsConn    net.Conn // <-- Store the connection
	aircraft   map[string]*sbs.Aircraft

	initialPositionFound bool // <-- 1. ADD THIS FLAG
	// ---------------

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
	headerMod := header.New()

	// Set the initial zoom on the footer
	footerMod.SetZoom(mapMod.GetZoomLevel())

	return model{
		headerModel: headerMod,
		mapModel:    mapMod,
		footerModel: footerMod,
		aircraft:    make(map[string]*sbs.Aircraft),
		// initialPositionFound is 'false' by default
	}
}

func (m model) Init() tea.Cmd {
	// Start BOTH the connection AND the render ticker
	return tea.Batch(
		sbs.ConnectCmd(),
		TickCmd(),
	)
}

// mergeAircraft is a helper to update the master aircraft list
func (m *model) mergeAircraft(update *sbs.Aircraft) {
	if update == nil {
		return
	}

	// Get or create aircraft in our master list
	ac, ok := m.aircraft[update.ICAO]
	if !ok {
		ac = update // This is the first time we see it
		m.aircraft[update.ICAO] = ac
		return
	}

	// Merge the new data
	if update.Callsign != "" {
		ac.Callsign = update.Callsign
	}
	if update.Lat != 0 && update.Lon != 0 {
		ac.Lat = update.Lat
		ac.Lon = update.Lon
	}
	if update.Speed != 0 {
		ac.Speed = update.Speed
	}
	if update.Track != 0 {
		ac.Track = update.Track
	}
	ac.LastSeen = time.Now()
}


func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// --- Global Error Handling ---
	if m.err != nil {
		if _, ok := msg.(tea.KeyMsg); ok {
			return m, tea.Quit
		}
		return m, nil
	}

	var (
		headerCmd tea.Cmd
		mapCmd    tea.Cmd
		footerCmd tea.Cmd
		cmds      []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// --- Layout ---
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

	// --- Handle SBS Messages ---
	case sbs.SbsConnectedMsg:
		m.sbsScanner = msg.Scanner
		m.sbsConn = msg.Conn // <-- Save the connection
		// Start listening for the first line
		cmds = append(cmds, sbs.WaitForSbsLine(m.sbsScanner))

	case sbs.SbsErrorMsg:
		m.err = msg.Err // Show the error
		return m, nil

	case sbs.AircraftUpdateMsg:
		// --- DATA LOOP ---
		m.mergeAircraft(msg.Update)

		// --- 2. ADD THIS AUTO-ZOOM BLOCK ---
		if !m.initialPositionFound && msg.Update != nil && msg.Update.Lat != 0 {
			m.initialPositionFound = true // Set flag
			// Tell the map to auto-zoom
			m.mapModel.SetViewToLocation(msg.Update.Lat, msg.Update.Lon)
			// Sync the footer's zoom level
			m.footerModel.SetZoom(m.mapModel.GetZoomLevel())
		}
		// --- END AUTO-ZOOM BLOCK ---

		// Ask for the next line (fast)
		cmds = append(cmds, sbs.WaitForSbsLine(m.sbsScanner))
		// --- We DO NOT update the map here ---

	// --- RENDER LOOP ---
	case TickMsg:
		// The render ticker fired.
		// 1. Tell the map to update with the *current* aircraft list
		m.mapModel.UpdateAircraft(m.aircraft)
		// 2. Ask for the next tick
		cmds = append(cmds, TickCmd())

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			// Cleanly close the connection
			if m.sbsConn != nil {
				m.sbsConn.Close()
			}
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
		m.headerModel, headerCmd = m.headerModel.Update(msg)
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
			"Error:\n\n" + m.err.Error() + // Simplified error
				"\n\nPress any key to quit.",
		)
	}

	// --- Normal View ---
	headerView := m.headerModel.View()
	mapView := m.mapModel.View()
	footerView := m.footerModel.View()

	// Stack them vertically
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