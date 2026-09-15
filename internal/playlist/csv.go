// Package playlist parses CSV playlist exports into normalized rows.
//
// The parser is deliberately forgiving about column order and extra columns:
// any of the known aliases for the track/artist/album fields is accepted, and
// unknown columns are ignored. This makes it compatible with Spotify exports
// ("My Spotify Library.csv" and similar) as well as hand-written playlists.
package playlist

import (
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Row is a single track parsed from the CSV.
type Row struct {
	Line         int
	TrackName    string
	ArtistName   string
	AlbumName    string
	PlaylistName string
	ISRC         string
	SpotifyID    string
	Type         string
	TrackNumber  *int
	DiscNumber   *int
	DurationMs   *int
}

// Warning describes a row that was skipped without failing the import.
type Warning struct {
	Line    int
	Message string
}

// ParseFile reads and parses a CSV playlist file.
func ParseFile(path string) ([]Row, []Warning, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	return parseBytes(data)
}

// Parse reads and parses a CSV playlist from r.
func Parse(r io.Reader) ([]Row, []Warning, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, fmt.Errorf("read csv: %w", err)
	}
	return parseBytes(data)
}

// columnMap stores the resolved index of every known column (-1 when absent).
type columnMap struct {
	track        int
	artist       int
	album        int
	playlistName int
	isrc         int
	spotifyID    int
	kind         int
	trackNumber  int
	discNumber   int
	durationMs   int
}

func parseBytes(data []byte) ([]Row, []Warning, error) {
	text, err := decodeText(data)
	if err != nil {
		return nil, nil, err
	}

	reader := csv.NewReader(strings.NewReader(text))
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	records, err := reader.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("parse csv: %w", err)
	}
	if len(records) == 0 {
		return nil, nil, errors.New("csv contains no header row")
	}

	cols, err := resolveColumns(records[0])
	if err != nil {
		return nil, nil, err
	}

	rows := make([]Row, 0, len(records)-1)
	var warnings []Warning
	for index, record := range records[1:] {
		line := index + 2
		row, ok, warning := buildRow(record, cols, line)
		if warning != nil {
			warnings = append(warnings, *warning)
		}
		if ok {
			rows = append(rows, row)
		}
	}
	return rows, warnings, nil
}

func resolveColumns(header []string) (columnMap, error) {
	cols := columnMap{-1, -1, -1, -1, -1, -1, -1, -1, -1, -1}
	for index, name := range header {
		target := columnTarget(normalizeHeader(name))
		switch target {
		case "track":
			cols.track = index
		case "artist":
			cols.artist = index
		case "album":
			cols.album = index
		case "playlist":
			cols.playlistName = index
		case "isrc":
			cols.isrc = index
		case "spotify_id":
			cols.spotifyID = index
		case "type":
			cols.kind = index
		case "track_number":
			cols.trackNumber = index
		case "disc_number":
			cols.discNumber = index
		case "duration_ms":
			cols.durationMs = index
		}
	}
	if cols.track < 0 {
		return cols, errors.New("csv is missing a track name column (expected 'Track name', 'Title' or 'Track')")
	}
	if cols.artist < 0 {
		return cols, errors.New("csv is missing an artist column (expected 'Artist name' or 'Artist')")
	}
	return cols, nil
}

var headerAliases = map[string]string{
	"track name": "track", "track": "track", "title": "track", "song": "track", "name": "track",
	"artist name": "artist", "artist": "artist", "artists": "artist",
	"album": "album", "album name": "album",
	"playlist name": "playlist", "playlist": "playlist",
	"isrc":         "isrc",
	"spotify - id": "spotify_id", "spotify id": "spotify_id", "spotify_id": "spotify_id", "spotify": "spotify_id",
	"type":         "type",
	"track number": "track_number", "track_number": "track_number", "track no": "track_number",
	"disc number": "disc_number", "disc_number": "disc_number", "disc no": "disc_number", "disc": "disc_number",
	"duration (ms)": "duration_ms", "duration_ms": "duration_ms", "duration": "duration_ms",
}

func columnTarget(header string) string {
	if target, ok := headerAliases[header]; ok {
		return target
	}
	return ""
}

func normalizeHeader(name string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(name, "\uFEFF")))
}

func buildRow(record []string, cols columnMap, line int) (Row, bool, *Warning) {
	track := cell(record, cols.track)
	artist := cell(record, cols.artist)

	if track == "" && artist == "" {
		return Row{}, false, nil // entirely blank row
	}
	if track == "" {
		return Row{}, false, &Warning{Line: line, Message: "missing track name"}
	}
	if artist == "" {
		return Row{}, false, &Warning{Line: line, Message: "missing artist name"}
	}

	return Row{
		Line:         line,
		TrackName:    track,
		ArtistName:   artist,
		AlbumName:    cell(record, cols.album),
		PlaylistName: cell(record, cols.playlistName),
		ISRC:         cell(record, cols.isrc),
		SpotifyID:    cell(record, cols.spotifyID),
		Type:         cell(record, cols.kind),
		TrackNumber:  parseOptionalInt(cell(record, cols.trackNumber)),
		DiscNumber:   parseOptionalInt(cell(record, cols.discNumber)),
		DurationMs:   parseOptionalInt(cell(record, cols.durationMs)),
	}, true, nil
}

func cell(record []string, index int) string {
	if index < 0 || index >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[index])
}

func parseOptionalInt(raw string) *int {
	if raw == "" {
		return nil
	}
	// Tolerate values such as "1/12" or "120.0" from other exporters.
	if slash := strings.IndexByte(raw, '/'); slash >= 0 {
		raw = raw[:slash]
	}
	if dot := strings.IndexByte(raw, '.'); dot >= 0 {
		raw = raw[:dot]
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return nil
	}
	return &value
}

func decodeText(data []byte) (string, error) {
	switch {
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}):
		return decodeUTF16(data[2:], binary.LittleEndian)
	case bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		return decodeUTF16(data[2:], binary.BigEndian)
	default:
		return strings.TrimPrefix(string(data), "\uFEFF"), nil
	}
}

func decodeUTF16(data []byte, order binary.ByteOrder) (string, error) {
	if len(data)%2 != 0 {
		return "", errors.New("invalid utf-16 input: odd number of bytes")
	}
	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = order.Uint16(data[i*2:])
	}
	return string(utf16.Decode(units)), nil
}
