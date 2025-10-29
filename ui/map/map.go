package mapview

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jonas-p/go-shp"

	"termtrack/sbs"
)

// Constants for Panning and Zooming
const (
	panFactor  = 0.1
	zoomFactor = 1.2
)

const airportShapePath = "airportdata/ne_10m_airports.shp"

// Model holds the map's state
type Model struct {
	width  int
	height int

	mapPolygons   []*shp.Polygon
	airportPoints []*shp.Point
	aircraft      map[string]*sbs.Aircraft
	originalBounds shp.Box
	viewBounds     shp.Box

	// --- Caching ---
	cachedStaticGrid [][]string
	needsRedraw      bool
	// ---------------
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

// New creates a new map model
func New(mapShapePath string) (Model, error) {
	// 1. Load polygons (map data)
	polygons, bounds, err := loadMapData(mapShapePath)
	if err != nil {
		return Model{}, err
	}

	// 2. Load points (airport data) from airports.go
	points, err := loadAirportData(airportShapePath)
	if err != nil {
		return Model{}, fmt.Errorf("failed to load airport data: %w", err)
	}

	return Model{
		mapPolygons:   polygons,
		airportPoints: points,
		aircraft:      make(map[string]*sbs.Aircraft),
		originalBounds: bounds,
		viewBounds:    bounds,
		width:         80,
		height:        23,
		needsRedraw:   true,
	}, nil
}

func (m Model) Init() tea.Cmd {
	return nil
}

// UpdateAircraft receives the master list from main.go
func (m *Model) UpdateAircraft(allAircraft map[string]*sbs.Aircraft) {
	m.aircraft = allAircraft
}

// ---
// 2. MODIFIED: This now centers and zooms to 25.5x
// ---
// SetViewToLocation centers and zooms the map on a specific lat/lon
func (m *Model) SetViewToLocation(lat, lon float64) {
	// Calculate the geographic aspect ratio of the *original* map
	origWidth := m.originalBounds.MaxX - m.originalBounds.MinX
	origHeight := m.originalBounds.MaxY - m.originalBounds.MinY
	origGeoAspect := origWidth / origHeight

	// Calculate the new geographic width based on the 25.5x zoom
	newGeoWidth := origWidth / 25.5

	// Calculate the new geographic height, maintaining the original aspect ratio
	newGeoHeight := newGeoWidth / origGeoAspect

	// Set the new view bounds, centered on the plane
	halfWidth := newGeoWidth / 2.0
	halfHeight := newGeoHeight / 2.0

	m.viewBounds.MinX = lon - halfWidth
	m.viewBounds.MaxX = lon + halfWidth
	m.viewBounds.MinY = lat - halfHeight
	m.viewBounds.MaxY = lat + halfHeight

	m.needsRedraw = true
}

// zoom zooms the viewBounds in or out, centered on the current view
func (m *Model) zoom(factor float64) {
	centerX := (m.viewBounds.MinX + m.viewBounds.MaxX) / 2
	centerY := (m.viewBounds.MinY + m.viewBounds.MaxY) / 2
	width := m.viewBounds.MaxX - m.viewBounds.MinX
	height := m.viewBounds.MaxY - m.viewBounds.MinY

	newWidth := width * factor
	newHeight := height * factor

	if newWidth > (m.originalBounds.MaxX-m.originalBounds.MinX) || newHeight > (m.originalBounds.MaxY-m.originalBounds.MinY) {
		m.viewBounds = m.originalBounds
		m.needsRedraw = true
		return
	}

	m.viewBounds.MinX = centerX - (newWidth / 2)
	m.viewBounds.MaxX = centerX + (newWidth / 2)
	m.viewBounds.MinY = centerY - (newHeight / 2)
	m.viewBounds.MaxY = centerY + (newHeight / 2)
	m.needsRedraw = true
}

// pan moves the viewBounds
func (m *Model) pan(dx, dy float64) {
	width := m.viewBounds.MaxX - m.viewBounds.MinX
	height := m.viewBounds.MaxY - m.viewBounds.MinY

	panX := width * dx
	panY := height * dy

	m.viewBounds.MinX += panX
	m.viewBounds.MaxX += panX
	m.viewBounds.MinY += panY
	m.viewBounds.MaxY += panY
	m.needsRedraw = true
}

// GetZoomLevel returns the current zoom factor
func (m Model) GetZoomLevel() float64 {
	if m.viewBounds.MaxX == m.viewBounds.MinX {
		return 1.0
	}
	// Calculate zoom based on the X-axis (width)
	return (m.originalBounds.MaxX - m.originalBounds.MinX) / (m.viewBounds.MaxX - m.viewBounds.MinX)
}

// Update handles key and window messages
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.needsRedraw = true

	case tea.KeyMsg:
		switch msg.String() {
		case "k", "up":
			m.pan(0, panFactor)
		case "l", "down":
			m.pan(0, -panFactor)
		case "j", "left":
			m.pan(-panFactor, 0)
		case ";", "right":
			m.pan(panFactor, 0)
		case "K":
			m.zoom(1 / zoomFactor)
		case "L":
			m.zoom(zoomFactor)
		case "r":
			m.viewBounds = m.originalBounds
			m.needsRedraw = true
		}
	}

	return m, nil
}

// ---
// 1. MODIFIED: This function now correctly handles aspect ratio
// ---
// project converts lon/lat to terminal x/y coordinates
func (m *Model) project(lon, lat float64, viewWidth, viewHeight int) (int, int) {
	if m.viewBounds.MaxX == m.viewBounds.MinX {
		m.viewBounds.MaxX += 1e-6
	}
	if m.viewBounds.MaxY == m.viewBounds.MinY {
		m.viewBounds.MaxY += 1e-6
	}

	// Normalize coordinates to [0, 1] based on the current view
	x := (lon - m.viewBounds.MinX) / (m.viewBounds.MaxX - m.viewBounds.MinX)
	y := (m.viewBounds.MaxY - lat) / (m.viewBounds.MaxY - m.viewBounds.MinY)

	// A value of 2.0 assumes chars are 2x tall as wide.
	// A smaller value (like 1.9 or 1.8) squashes the map less.
	// You said 2.0 was too wide, so I'm using 1.9.
	const charAspect = 1.9

	// We DIVIDE x by the aspect ratio to "squash" the wide horizontal axis
	tuiX := int(x * float64(viewWidth) / charAspect)
	tuiY := int(y * float64(viewHeight))
	return tuiX, tuiY
}

// copyGrid duplicates a 2D string slice
func (m *Model) copyGrid(source [][]string) [][]string {
	if source == nil {
		return nil
	}

	height := len(source)
	if height == 0 {
		return [][]string{}
	}
	width := len(source[0])
	if width == 0 {
		return make([][]string, height)
	}

	dest := make([][]string, height)
	for i := range dest {
		dest[i] = make([]string, width)
		copy(dest[i], source[i])
	}
	return dest
}

// renderMapViewport generates ASCII map
func (m *Model) renderMapViewport(viewWidth, viewHeight int) string {
	if viewWidth <= 0 {
		viewWidth = 1
	}
	if viewHeight <= 0 {
		viewHeight = 1
	}

	// --- Define styles for map elements ---
	mapStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))     // Bright White
	airportStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("220")) // Yellow
	planeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("81"))  // Bright Purple/Blue
	callsignStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("86")) // Cyan


	// --- 1. Render static map only once or on pan/zoom ---
	if m.needsRedraw || m.cachedStaticGrid == nil || len(m.cachedStaticGrid) != viewHeight || len(m.cachedStaticGrid[0]) != viewWidth {

		grid := make([][]string, viewHeight)
		for i := range grid {
			grid[i] = make([]string, viewWidth)
			for j := range grid[i] {
				grid[i][j] = " "
			}
		}

		// Draw Polygons
		for _, polygon := range m.mapPolygons {
			polyBounds := polygon.BBox()
			if polyBounds.MaxX < m.viewBounds.MinX ||
				polyBounds.MinX > m.viewBounds.MaxX ||
				polyBounds.MaxY < m.viewBounds.MinY ||
				polyBounds.MinY > m.viewBounds.MaxY {
				continue
			}

			step := 3
			for i := 0; i < len(polygon.Points); i += step {
				point := polygon.Points[i]
				x, y := m.project(point.X, point.Y, viewWidth, viewHeight)
				if x >= 0 && x < viewWidth && y >= 0 && y < viewHeight {
					grid[y][x] = mapStyle.Render(".")
				}
			}
		}

		// Draw Airports
		for _, point := range m.airportPoints {
			x, y := m.project(point.X, point.Y, viewWidth, viewHeight)
			if x >= 0 && x < viewWidth && y >= 0 && y < viewHeight {
				grid[y][x] = airportStyle.Render("*")
			}
		}

		// Save static grid to cache
		m.cachedStaticGrid = grid
		m.needsRedraw = false
	}

	// --- 2. Copy cached static grid ---
	grid := m.copyGrid(m.cachedStaticGrid)
	if grid == nil { // Failsafe
		grid = make([][]string, viewHeight)
		for i := range grid { grid[i] = make([]string, viewWidth) }
	}

	// --- 3. Draw Aircraft (Icons, then Labels) ---

	// Pass 1: Draw plane icons and store their positions
	type planePosition struct {
		x int
		y int
	}
	planePositions := make(map[string]planePosition) // ICAO -> position

	for icao, ac := range m.aircraft {
		if ac.Lat == 0 && ac.Lon == 0 {
			continue
		}
		x, y := m.project(ac.Lon, ac.Lat, viewWidth, viewHeight)
		if x >= 0 && x < viewWidth && y >= 0 && y < viewHeight {
			grid[y][x] = planeStyle.Render("✈")
			planePositions[icao] = planePosition{x: x, y: y}
		}
	}

	// Pass 2: Draw callsigns under the icons
	for icao, pos := range planePositions {
		ac := m.aircraft[icao] // Get the full aircraft data
		if ac.Callsign == "" {
			continue // No callsign to draw
		}

		// Calculate position for the callsign (one row below)
		yi := pos.y + 1

		// Stop if the callsign row is off-screen
		if yi >= viewHeight {
			continue
		}

		// Draw the callsign character by character
		callsignRunes := []rune(ac.Callsign)
		for i, r := range callsignRunes {
			xi := pos.x + i // Start at the same X as the plane

			// Stop if we go off the right side of the screen
			if xi >= viewWidth {
				break
			}

			// Only draw if the cell is empty (so we don't overwrite map lines)
			if grid[yi][xi] == " " {
				grid[yi][xi] = callsignStyle.Render(string(r))
			}
		}
	}

	// --- 4. Convert to string ---
	var b strings.Builder
	for _, row := range grid {
		b.WriteString(strings.Join(row, ""))
		b.WriteRune('\n')
	}
	return b.String()
}


func (m Model) View() string {
	mapStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Width(m.width - 2).
		Height(m.height - 2)

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

	return mapStyle.Render(mapContent)
}