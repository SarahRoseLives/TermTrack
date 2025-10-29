package mapview

import (
	"fmt"

	"github.com/jonas-p/go-shp"
)

// loadAirportData reads the airport shapefile and returns a slice of points.
func loadAirportData(path string) ([]*shp.Point, error) {
	shapeFile, err := shp.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open airport shapefile: %w", err)
	}
	defer shapeFile.Close()

	var points []*shp.Point
	for shapeFile.Next() {
		_, shape := shapeFile.Shape()
		point, ok := shape.(*shp.Point)
		if !ok {
			continue // Skip if it's not a point (e.g., polygon, polyline)
		}
		points = append(points, point)
	}

	if len(points) == 0 {
		// This isn't a critical error, but good to be aware of
		return nil, fmt.Errorf("no points found in airport shapefile: %s", path)
	}

	return points, nil
}