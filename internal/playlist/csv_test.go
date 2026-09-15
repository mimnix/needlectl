package playlist

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf16"
)

const spotifyCSV = "\ufeffTrack name,Artist name,Album,Playlist name,Type,ISRC,Spotify - id\n" +
	"\"Disorder\",\"神崎エルザ\",\"Elza\",\"Anime\",\"Playlist\",\"JPU901801170\",\"4uOGiGwI26L2jrJ1x2wsMh\"\n" +
	"\"Hello, World\",\"Artist\",\"Album\",\"Anime\",\"Playlist\",\"ISRC2\",\"id2\"\n"

func TestParseSpotifyExportWithBOM(t *testing.T) {
	rows, warnings, err := Parse(strings.NewReader(spotifyCSV))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}

	first := rows[0]
	if first.TrackName != "Disorder" || first.ArtistName != "神崎エルザ" || first.AlbumName != "Elza" {
		t.Errorf("first row = %+v, want the Disorder track", first)
	}
	if first.ISRC != "JPU901801170" || first.SpotifyID != "4uOGiGwI26L2jrJ1x2wsMh" {
		t.Errorf("first row identifiers = %+v", first)
	}
	if rows[1].TrackName != "Hello, World" {
		t.Errorf("quoted comma not preserved: %q", rows[1].TrackName)
	}
}

func TestParseAcceptsAlternateColumnNames(t *testing.T) {
	input := "Title,Artist,Album name\nSong,Tester,Record\n"
	rows, _, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(rows) != 1 || rows[0].TrackName != "Song" || rows[0].ArtistName != "Tester" || rows[0].AlbumName != "Record" {
		t.Fatalf("rows = %+v, want the Song row", rows)
	}
}

func TestParseHandlesCRLFAndBlankRows(t *testing.T) {
	input := "Track name,Artist name,Album\r\nA,B,C\r\n,\r\nD,E,F\r\n"
	rows, warnings, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	if rows[0].TrackName != "A" || rows[1].TrackName != "D" {
		t.Errorf("rows = %+v", rows)
	}
}

func TestParseWarnsOnRowWithoutArtist(t *testing.T) {
	input := "Track name,Artist name,Album\n\"A\",\"\",\"C\"\n"
	rows, warnings, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("len(rows) = %d, want 0", len(rows))
	}
	if len(warnings) != 1 || warnings[0].Line != 2 {
		t.Fatalf("warnings = %+v, want one warning on line 2", warnings)
	}
}

func TestParseFailsWhenTrackColumnIsMissing(t *testing.T) {
	_, _, err := Parse(strings.NewReader("Artist name,Album\nA,B\n"))
	if err == nil || !strings.Contains(err.Error(), "track") {
		t.Fatalf("error = %v, want a missing track column error", err)
	}
}

func TestParseFailsWhenArtistColumnIsMissing(t *testing.T) {
	_, _, err := Parse(strings.NewReader("Track name,Album\nA,B\n"))
	if err == nil || !strings.Contains(err.Error(), "artist") {
		t.Fatalf("error = %v, want a missing artist column error", err)
	}
}

func TestParseReadsOptionalNumericColumns(t *testing.T) {
	input := "Track name,Artist name,Track number,Disc number,Duration (ms)\nA,B,3/12,2,215000\n"
	rows, _, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	row := rows[0]
	if row.TrackNumber == nil || *row.TrackNumber != 3 {
		t.Errorf("TrackNumber = %v, want 3", row.TrackNumber)
	}
	if row.DiscNumber == nil || *row.DiscNumber != 2 {
		t.Errorf("DiscNumber = %v, want 2", row.DiscNumber)
	}
	if row.DurationMs == nil || *row.DurationMs != 215000 {
		t.Errorf("DurationMs = %v, want 215000", row.DurationMs)
	}
}

func TestParseDecodesUTF16(t *testing.T) {
	content := "Track name,Artist name,Album\nSong,Tester,Record\n"
	rows, _, err := Parse(bytes.NewReader(encodeUTF16LE(content)))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(rows) != 1 || rows[0].TrackName != "Song" {
		t.Fatalf("rows = %+v, want the Song row", rows)
	}
}

func encodeUTF16LE(s string) []byte {
	units := utf16.Encode([]rune(s))
	out := []byte{0xFF, 0xFE}
	for _, unit := range units {
		out = append(out, byte(unit), byte(unit>>8))
	}
	return out
}

// TestParseSpotifyExportFixture guards the exact format of a Spotify library
// export using a committed synthetic fixture (never a personal file).
func TestParseSpotifyExportFixture(t *testing.T) {
	rows, warnings, err := ParseFile("testdata/spotify-export.csv")
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3", len(rows))
	}
	if rows[0].TrackName != "Disorder" || rows[0].ArtistName != "神崎エルザ" || rows[0].AlbumName != "Elza" {
		t.Errorf("first row = %+v", rows[0])
	}
	if rows[1].TrackName != "Hello, World" {
		t.Errorf("quoted comma not preserved: %q", rows[1].TrackName)
	}
}
