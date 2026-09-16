package tracks

import (
	"errors"
	"testing"
)

func TestResolveReturnsUniqueExactTitle(t *testing.T) {
	candidates := []Track{
		{ID: "a", Title: "Disorder", ArtistName: "神崎エルザ", AlbumName: "Elza"},
		{ID: "b", Title: "Rising Hope", ArtistName: "LiSA", AlbumName: "Launcher"},
	}

	track, err := Resolve("disorder", "", "", candidates)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if track.ID != "a" {
		t.Errorf("resolved %q, want a", track.ID)
	}
}

func TestResolveReportsAmbiguousExactTitles(t *testing.T) {
	candidates := []Track{
		{ID: "a", Title: "Intro", ArtistName: "LiSA", AlbumName: "Launcher"},
		{ID: "b", Title: "Intro", ArtistName: "LiSA", AlbumName: "Other"},
	}

	_, err := Resolve("Intro", "", "", candidates)

	var ambiguous *AmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("error = %v, want *AmbiguousError", err)
	}
	if len(ambiguous.Matches) != 2 {
		t.Errorf("matches = %d, want 2", len(ambiguous.Matches))
	}
}

func TestResolveArtistNarrowsAmbiguity(t *testing.T) {
	candidates := []Track{
		{ID: "a", Title: "Intro", ArtistName: "LiSA", AlbumName: "Launcher"},
		{ID: "b", Title: "Intro", ArtistName: "Someone Else", AlbumName: "Other"},
	}

	track, err := Resolve("Intro", "Someone Else", "", candidates)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if track.ID != "b" {
		t.Errorf("resolved %q, want b", track.ID)
	}
}

func TestResolveAlbumNarrowsAmbiguity(t *testing.T) {
	candidates := []Track{
		{ID: "a", Title: "Disorder", ArtistName: "神崎エルザ", AlbumName: "Elza"},
		{ID: "b", Title: "Disorder", ArtistName: "神崎エルザ", AlbumName: "Other"},
	}

	track, err := Resolve("Disorder", "神崎エルザ", "Other", candidates)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if track.ID != "b" {
		t.Errorf("resolved %q, want b", track.ID)
	}
}

func TestResolveFuzzyTitle(t *testing.T) {
	candidates := []Track{
		{ID: "a", Title: "Haruka Mirai", ArtistName: "KANKAKU PIERO", AlbumName: "Iroirotoiro"},
	}

	track, err := Resolve("Harukamirai", "", "", candidates)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if track.ID != "a" {
		t.Errorf("resolved %q, want a", track.ID)
	}
}

func TestResolveRefusesToGuessOnEqualScores(t *testing.T) {
	candidates := []Track{
		{ID: "a", Title: "Haruka Mirai", ArtistName: "KANKAKU PIERO"},
		{ID: "b", Title: "Haruka Mirai", ArtistName: "KANKAKU PIERO"},
	}

	if _, err := Resolve("Harukamirai", "", "", candidates); err == nil {
		t.Fatal("Resolve returned nil error, want an ambiguity refusal")
	}
}

func TestResolveReturnsNoMatchForUnrelatedTitle(t *testing.T) {
	candidates := []Track{
		{ID: "a", Title: "Disorder", ArtistName: "神崎エルザ"},
	}

	if _, err := Resolve("Completely Different", "", "", candidates); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("error = %v, want ErrNoMatch", err)
	}
}

func TestResolveReturnsNoMatchForUnknownArtist(t *testing.T) {
	candidates := []Track{
		{ID: "a", Title: "Disorder", ArtistName: "神崎エルザ"},
	}

	if _, err := Resolve("Disorder", "Nobody At All", "", candidates); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("error = %v, want ErrNoMatch", err)
	}
}

func TestResolveReturnsNoMatchForEmptyCandidates(t *testing.T) {
	if _, err := Resolve("Disorder", "", "", nil); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("error = %v, want ErrNoMatch", err)
	}
}
