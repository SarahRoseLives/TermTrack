package sbs

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const sbsAddress = "localhost:30003"

// Aircraft holds the state of a single aircraft
type Aircraft struct {
	ICAO     string
	Callsign string
	Lat      float64
	Lon      float64
	Speed    float64
	Track    float64
	LastSeen time.Time
}

// SbsConnectedMsg is sent when we successfully connect to the feed
type SbsConnectedMsg struct {
	Scanner *bufio.Scanner
	Conn    net.Conn // <-- ADD THIS
}

// SbsErrorMsg is sent when a connection or parsing error occurs
type SbsErrorMsg struct {
	Err error
}

// AircraftUpdateMsg is sent for a single parsed SBS line
// We use a pointer so we can merge fields in main.go
type AircraftUpdateMsg struct {
	Update *Aircraft
}

// ConnectCmd returns a command that attempts to connect to the SBS feed
func ConnectCmd() tea.Cmd {
	return func() tea.Msg {
		conn, err := net.Dial("tcp", sbsAddress)
		if err != nil {
			return SbsErrorMsg{Err: fmt.Errorf("sbs connect: %w", err)}
		}

		scanner := bufio.NewScanner(conn)
		return SbsConnectedMsg{Scanner: scanner, Conn: conn} // <-- PASS CONN HERE
	}
}

// WaitForSbsLine returns a command that waits for the next line from the scanner
func WaitForSbsLine(scanner *bufio.Scanner) tea.Cmd {
	return func() tea.Msg {
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return SbsErrorMsg{Err: fmt.Errorf("sbs read: %w", err)}
			}
			return SbsErrorMsg{Err: fmt.Errorf("sbs feed disconnected")}
		}

		line := scanner.Text()

		// Parse and return the single update
		if acUpdate := parseSbsLine(line); acUpdate != nil {
			return AircraftUpdateMsg{Update: acUpdate}
		}

		// If parseSbsLine returned nil, we send nil.
		return AircraftUpdateMsg{Update: nil}
	}
}

// parseSbsLine attempts to parse a single line into an *Aircraft struct
func parseSbsLine(line string) *Aircraft {
	fields := strings.Split(line, ",")
	if len(fields) < 11 || fields[0] != "MSG" {
		return nil // Not a message, or too short, ignore
	}

	msgType := fields[1]
	icao := fields[4]

	if icao == "" {
		return nil // No ICAO, can't track
	}

	// Create a partial update.
	update := &Aircraft{
		ICAO:     icao,
		LastSeen: time.Now(),
	}

	switch msgType {
	case "1": // Callsign
		if len(fields) >= 11 {
			update.Callsign = strings.TrimSpace(fields[10])
		}
	case "3": // Position
		if len(fields) >= 16 {
			if lat, err := strconv.ParseFloat(fields[14], 64); err == nil {
				update.Lat = lat
			}
			if lon, err := strconv.ParseFloat(fields[15], 64); err == nil {
				update.Lon = lon
			}
		}
	case "4": // Velocity
		if len(fields) >= 14 {
			if spd, err := strconv.ParseFloat(fields[11], 64); err == nil {
				update.Speed = spd
			}
			if trk, err := strconv.ParseFloat(fields[12], 64); err == nil {
				update.Track = trk
			}
		}
	default:
		return nil // We don't care about this message type
	}

	// Only return if we actually got useful data (callsign, pos, or vel)
	if update.Callsign != "" || update.Lat != 0 || update.Speed != 0 {
		return update
	}
	return nil
}