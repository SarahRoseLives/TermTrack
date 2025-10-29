package main

import (
	"fmt"
	"log"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jonas-p/go-shp"
)

// mapShapePath is the path to your downloaded shapefile
const mapShapePath = "mapdata/ne_10m_admin_1_states_provinces.shp"

// Constants for Panning and Zooming
const (
	panFactor  = 0.1 // Pan 10% of the current view width/height
	zoomFactor = 1.2 // Zoom in/out by 20%
)

// model holds the application's state
type model struct {
	width  int // Terminal width
	height int // Terminal height

	mapPolygons    []*shp.Polygon // Polygons from shapefile
	originalBounds shp.Box        // The bounds of the entire map, never changes
	viewBounds     shp.Box        // The bounds of the current viewport (pans and zooms)
	err            error          // Store any errors
}

// loadMapData reads the shapefile and computes bounding box manually
func loadMapData(path string) ([]*shp.Polygon, shp.Box, error) {
	shapeFile, err := shp.Open(path)
	if err != nil {
		return nil, shp.Box{}, fmt.Errorf("failed to open shapefile: %w", err)
	}
	defer shapeFile.Close()

	var polygons []*shp.Polygon
	bounds := shp.Box{
		MinX: 1e9,
		MinY: 1e9,
		MaxX: -1e9,
		MaxY: -1e9,
	}

	for shapeFile.Next() {
		_, shape := shapeFile.Shape()
		polygon, ok := shape.(*shp.Polygon)
		if !ok {
			continue
		}
		polygons = append(polygons, polygon)

		// Compute bounding box manually
		for _, p := range polygon.Points {
			if p.X < bounds.MinX {
				bounds.MinX = p.X
			}
			if p.X > bounds.MaxX {
				bounds.MaxX = p.X
			}
			if p.Y < bounds.MinY {
				bounds.MinY = p.Y
			}
			if p.Y > bounds.MaxY {
				bounds.MaxY = p.Y
			}
		}
	}

	if len(polygons) == 0 {
		return nil, shp.Box{}, fmt.Errorf("no polygons found in shapefile")
	}

	return polygons, bounds, nil
}

// initialModel creates the starting model
func initialModel() model {
	polygons, bounds, err := loadMapData(mapShapePath)
	if err != nil {
		return model{err: err}
	}

	return model{
		mapPolygons:    polygons,
		originalBounds: bounds, // Store the original bounds
		viewBounds:     bounds, // The view starts fully zoomed out
		width:          80,     // Default width
		height:         24,     // Default height
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

// zoom zooms the viewBounds in or out, centered on the current view
func (m *model) zoom(factor float64) {
	// Get current center and dimensions
	centerX := (m.viewBounds.MinX + m.viewBounds.MaxX) / 2
	centerY := (m.viewBounds.MinY + m.viewBounds.MaxY) / 2
	width := m.viewBounds.MaxX - m.viewBounds.MinX
	height := m.viewBounds.MaxY - m.viewBounds.MinY

	// Calculate new dimensions
	newWidth := width * factor
	newHeight := height * factor

	// Don't zoom out further than the original map
	if newWidth > (m.originalBounds.MaxX-m.originalBounds.MinX) || newHeight > (m.originalBounds.MaxY-m.originalBounds.MinY) {
		m.viewBounds = m.originalBounds
		return
	}

	// Set new bounds centered on the same point
	m.viewBounds.MinX = centerX - (newWidth / 2)
	m.viewBounds.MaxX = centerX + (newWidth / 2)
	m.viewBounds.MinY = centerY - (newHeight / 2)
	m.viewBounds.MaxY = centerY + (newHeight / 2)
}

// pan moves the viewBounds
func (m *model) pan(dx, dy float64) {
	width := m.viewBounds.MaxX - m.viewBounds.MinX
	height := m.viewBounds.MaxY - m.viewBounds.MinY

	// Calculate pan delta
	panX := width * dx
	panY := height * dy

	// Apply pan
	m.viewBounds.MinX += panX
	m.viewBounds.MaxX += panX
	m.viewBounds.MinY += panY
	m.viewBounds.MaxY += panY
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.err != nil {
		if _, ok := msg.(tea.KeyMsg); ok {
			return m, tea.Quit
		}
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit

		// --- Panning ---
		case "k", "up":
			m.pan(0, panFactor) // Pan Up (move map down, so increase Y)
		case "l", "down":
			m.pan(0, -panFactor) // Pan Down (move map up, so decrease Y)
		case "j", "left":
			m.pan(-panFactor, 0) // Pan Left (move map right, so decrease X)
		case ";", "right":
			m.pan(panFactor, 0) // Pan Right (move map left, so increase X)

		// --- Zooming ---
		case "K": // Zoom In (Shift+k)
			m.zoom(1 / zoomFactor) // Zoom in (divide by factor)
		case "L": // Zoom Out (Shift+l)
			m.zoom(zoomFactor) // Zoom out (multiply by factor)

		// --- Reset ---
		case "r":
			m.viewBounds = m.originalBounds
		}
	}

	return m, nil
}

// project converts lon/lat to terminal x/y coordinates
func (m *model) project(lon, lat float64, viewWidth, viewHeight int) (int, int) {
	// Project based on the *current viewBounds*, not the original bounds

	// Avoid divide by zero if perfectly zoomed in
	if m.viewBounds.MaxX == m.viewBounds.MinX {
		m.viewBounds.MaxX += 1e-6
	}
	if m.viewBounds.MaxY == m.viewBounds.MinY {
		m.viewBounds.MaxY += 1e-6
	}

	// Normalize coordinates to [0, 1] based on the current view
	x := (lon - m.viewBounds.MinX) / (m.viewBounds.MaxX - m.viewBounds.MinX)
	y := (m.viewBounds.MaxY - lat) / (m.viewBounds.MaxY - m.viewBounds.MinY)

	// Scale to TUI dimensions
	tuiX := int(x * float64(viewWidth))
	tuiY := int(y * float64(viewHeight))
	return tuiX, tuiY
}

// renderMapViewport generates ASCII map
func (m model) renderMapViewport(viewWidth, viewHeight int) string {
	if viewWidth <= 0 {
		viewWidth = 1 // Avoid zero-size grid
	}
	if viewHeight <= 0 {
		viewHeight = 1 // Avoid zero-size grid
	}

	grid := make([][]rune, viewHeight)
	for i := range grid {
		grid[i] = make([]rune, viewWidth)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}

	for _, polygon := range m.mapPolygons {
		// --- FIX: Call polygon.BBox() as a method ---
		polyBounds := polygon.BBox()
		if polyBounds.MaxX < m.viewBounds.MinX ||
			polyBounds.MinX > m.viewBounds.MaxX ||
			polyBounds.MaxY < m.viewBounds.MinY ||
			polyBounds.MinY > m.viewBounds.MaxY {
			continue // Skip this polygon, it's not in view
		}

		for _, point := range polygon.Points {
			x, y := m.project(point.X, point.Y, viewWidth, viewHeight)
			// Draw if projected point is within the TUI grid
			if x >= 0 && x < viewWidth && y >= 0 && y < viewHeight {
				grid[y][x] = '.'
			}
		}
	}

	var b strings.Builder
	for _, row := range grid {
		b.WriteString(string(row))
		b.WriteRune('\n')
	}
	return b.String()
}

func (m model) View() string {
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

	mapStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Width(m.width - 2).
		Height(m.height - 4)

	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Padding(0, 1)

	hBorders := mapStyle.GetBorderLeftSize() + mapStyle.GetBorderRightSize()
	vBorders := mapStyle.GetBorderTopSize() + mapStyle.GetBorderBottomSize()
	hPadding := mapStyle.GetPaddingLeft() + mapStyle.GetPaddingRight()
	vPadding := mapStyle.GetPaddingTop() + mapStyle.GetPaddingBottom()

	mapViewWidth := mapStyle.GetWidth() - hBorders - hPadding
	mapViewHeight := mapStyle.GetHeight() - vBorders - vPadding

	if mapViewWidth <= 0 {
		mapViewWidth = 1
	}
	if mapViewHeight <= 0 {
		mapViewHeight = 1
	}

	mapContent := m.renderMapViewport(mapViewWidth, mapViewHeight)
	mapView := mapStyle.Render(mapContent)

	// --- Footer ---

	// Calculate zoom level
	zoomLevel := (m.originalBounds.MaxX - m.originalBounds.MinX) / (m.viewBounds.MaxX - m.viewBounds.MinX)

	footerLeft := footerStyle.Render(fmt.Sprintf(
		"TermTrack | Map: %s | Zoom: %.1fx",
		mapShapePath, zoomLevel,
	))

	footerHelp := "Pan: j/k/l/; | Zoom: K/L | Reset: r | Quit: q"

	footerRight := footerStyle.Width(m.width - lipgloss.Width(footerLeft) - 1).
		Align(lipgloss.Right).
		Render(footerHelp)

	footerView := lipgloss.JoinHorizontal(lipgloss.Left, footerLeft, footerRight)

	return lipgloss.JoinVertical(lipgloss.Left, mapView, footerView)
}

func main() {
	// Simplified main, matching your original
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatalf("Alas, there's been an error: %v", err)
	}
}