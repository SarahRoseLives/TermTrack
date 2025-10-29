package main

import (
	"bufio"
	"log"
	"time"

	"termtrack/sbs" // <-- 1. Import the new sbs package
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
	sbsScanner         *bufio.Scanner             // <-- 2. Hold the TCP scanner
	aircraft           map[string]*sbs.Aircraft // <-- 3. Master aircraft list
	initialPositionFound bool                       // <-- 4. For auto-zoom
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
		aircraft:    make(map[string]*sbs.Aircraft), // <-- 5. Initialize the map
	}
}

func (m model) Init() tea.Cmd {
	return sbs.ConnectCmd() // <-- 6. Start the SBS connection on init
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

	// --- 7. Handle SBS Messages ---
	case sbs.SbsConnectedMsg:
		m.sbsScanner = msg.Scanner
		// Start listening for the first line
		cmds = append(cmds, sbs.WaitForSbsLine(m.sbsScanner))

	case sbs.SbsErrorMsg:
		m.err = msg.Err // Show the error
		return m, nil

	case sbs.AircraftUpdateMsg:
		if msg.Update != nil {
			update := msg.Update

			// Get or create aircraft in our master list
			ac, ok := m.aircraft[update.ICAO]
			if !ok {
				ac = update // This is the first time we see it
				m.aircraft[update.ICAO] = ac
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

			// --- Handle Auto-Zoom ---
			if ac.Lat != 0 && !m.initialPositionFound {
				m.initialPositionFound = true
				// Tell the map to zoom to this location
				m.mapModel.SetViewToLocation(ac.Lat, ac.Lon)
				// Update footer's zoom level
				m.footerModel.SetZoom(m.mapModel.GetZoomLevel())
			}

			// Pass the *entire* updated map to the component
			m.mapModel.UpdateAircraft(m.aircraft)
		}

		// Always listen for the next line
		cmds = append(cmds, sbs.WaitForSbsLine(m.sbsScanner))
	// --- End of SBS Handling ---


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