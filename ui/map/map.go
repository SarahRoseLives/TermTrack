package mapview

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jonas-p/go-shp"

	"termtrack/sbs" // <-- 1. Import sbs package for the Aircraft struct
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
	aircraft      map[string]*sbs.Aircraft // <-- 2. Add aircraft map
	originalBounds shp.Box
	viewBounds     shp.Box
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

	// 2. Load points (airport data)
	points, err := loadAirportData(airportShapePath)
	if err != nil {
		return Model{}, fmt.Errorf("failed to load airport data: %w", err)
	}

	return Model{
		mapPolygons:    polygons,
		airportPoints:  points,
		aircraft:       make(map[string]*sbs.Aircraft), // <-- 3. Initialize map
		originalBounds: bounds,
		viewBounds:     bounds,
		width:          80,
		height:         23,
	}, nil
}

func (m Model) Init() tea.Cmd {
	return nil
}

// --- 4. New methods for aircraft and auto-zoom ---

// UpdateAircraft receives the master list from main.go
func (m *Model) UpdateAircraft(allAircraft map[string]*sbs.Aircraft) {
	m.aircraft = allAircraft
}

// SetViewToLocation centers and zooms the map on a specific lat/lon
func (m *Model) SetViewToLocation(lat, lon float64) {
	// Set a small (zoomed-in) bounding box
	// 0.5 degrees is an arbitrary "zoom" level
	const zoomSize = 0.5
	m.viewBounds.MinX = lon - zoomSize
	m.viewBounds.MaxX = lon + zoomSize
	m.viewBounds.MinY = lat - zoomSize
	m.viewBounds.MaxY = lat + zoomSize
}

// --- End of new methods ---


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
		return
	}

	m.viewBounds.MinX = centerX - (newWidth / 2)
	m.viewBounds.MaxX = centerX + (newWidth / 2)
	m.viewBounds.MinY = centerY - (newHeight / 2)
	m.viewBounds.MaxY = centerY + (newHeight / 2)
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
}

// GetZoomLevel returns the current zoom factor
func (m Model) GetZoomLevel() float64 {
	if m.viewBounds.MaxX == m.viewBounds.MinX {
		return 1.0
	}
	return (m.originalBounds.MaxX - m.originalBounds.MinX) / (m.viewBounds.MaxX - m.viewBounds.MinX)
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

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
		}
	}

	return m, nil
}

// project converts lon/lat to terminal x/y coordinates
func (m *Model) project(lon, lat float64, viewWidth, viewHeight int) (int, int) {
	if m.viewBounds.MaxX == m.viewBounds.MinX {
		m.viewBounds.MaxX += 1e-6
	}
	if m.viewBounds.MaxY == m.viewBounds.MinY {
		m.viewBounds.MaxY += 1e-6
	}

	x := (lon - m.viewBounds.MinX) / (m.viewBounds.MaxX - m.viewBounds.MinX)
	y := (m.viewBounds.MaxY - lat) / (m.viewBounds.MaxY - m.viewBounds.MinY)

	tuiX := int(x * float64(viewWidth))
	tuiY := int(y * float64(viewHeight))
	return tuiX, tuiY
}

// renderMapViewport generates ASCII map
func (m Model) renderMapViewport(viewWidth, viewHeight int) string {
	if viewWidth <= 0 {
		viewWidth = 1
	}
	if viewHeight <= 0 {
		viewHeight = 1
	}

	grid := make([][]rune, viewHeight)
	for i := range grid {
		grid[i] = make([]rune, viewWidth)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}

	// 1. Draw Map Polygons
	for _, polygon := range m.mapPolygons {
		polyBounds := polygon.BBox()
		if polyBounds.MaxX < m.viewBounds.MinX ||
			polyBounds.MinX > m.viewBounds.MaxX ||
			polyBounds.MaxY < m.viewBounds.MinY ||
			polyBounds.MinY > m.viewBounds.MaxY {
			continue
		}

		for _, point := range polygon.Points {
			x, y := m.project(point.X, point.Y, viewWidth, viewHeight)
			if x >= 0 && x < viewWidth && y >= 0 && y < viewHeight {
				grid[y][x] = '.'
			}
		}
	}

	// 2. Draw Airports (on top of map)
	for _, point := range m.airportPoints {
		x, y := m.project(point.X, point.Y, viewWidth, viewHeight)
		if x >= 0 && x < viewWidth && y >= 0 && y < viewHeight {
			grid[y][x] = '*'
		}
	}

	// 3. Draw Aircraft (on top of airports and map)
	// <-- 5. Add render loop for aircraft
	for _, ac := range m.aircraft {
		// Only draw if we have a position
		if ac.Lat == 0 && ac.Lon == 0 {
			continue
		}

		x, y := m.project(ac.Lon, ac.Lat, viewWidth, viewHeight)
		if x >= 0 && x < viewWidth && y >= 0 && y < viewHeight {
			grid[y][x] = '✈' // Use a plane emoji
		}
	}
	// --- End of new section ---


	var b strings.Builder
	for _, row := range grid {
		b.WriteString(string(row))
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