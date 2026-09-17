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

	track, err := Resolve("disorder", Criteria{}, candidates)
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

	_, err := Resolve("Intro", Criteria{}, candidates)

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

	track, err := Resolve("Intro", Criteria{Artist: "Someone Else"}, candidates)
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

	track, err := Resolve("Disorder", Criteria{Artist: "神崎エルザ", Album: "Other"}, candidates)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if track.ID != "b" {
		t.Errorf("resolved %q, want b", track.ID)
	}
}

func TestResolveTrackNumberNarrowsAmbiguityWithinAnAlbum(t *testing.T) {
	candidates := []Track{
		{ID: "a", Title: "Intro", ArtistName: "LiSA", AlbumName: "Launcher", DiscNumber: 1, TrackNumber: 1},
		{ID: "b", Title: "Intro", ArtistName: "LiSA", AlbumName: "Launcher", DiscNumber: 1, TrackNumber: 3},
	}

	track, err := Resolve("Intro", Criteria{Track: 3}, candidates)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if track.ID != "b" {
		t.Errorf("resolved %q, want b", track.ID)
	}
}

func TestResolveDiscAndTrackNarrowAmbiguity(t *testing.T) {
	candidates := []Track{
		{ID: "a", Title: "Intro", ArtistName: "LiSA", AlbumName: "Launcher", DiscNumber: 1, TrackNumber: 1},
		{ID: "b", Title: "Intro", ArtistName: "LiSA", AlbumName: "Launcher", DiscNumber: 2, TrackNumber: 1},
	}

	track, err := Resolve("Intro", Criteria{Track: 1, Disc: 2}, candidates)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if track.ID != "b" {
		t.Errorf("resolved %q, want b", track.ID)
	}
}

func TestResolveTrackNumberWithoutDiscStaysAmbiguous(t *testing.T) {
	candidates := []Track{
		{ID: "a", Title: "Intro", ArtistName: "LiSA", AlbumName: "Launcher", DiscNumber: 1, TrackNumber: 1},
		{ID: "b", Title: "Intro", ArtistName: "LiSA", AlbumName: "Launcher", DiscNumber: 2, TrackNumber: 1},
	}

	_, err := Resolve("Intro", Criteria{Track: 1}, candidates)

	var ambiguous *AmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("error = %v, want *AmbiguousError", err)
	}
	if len(ambiguous.Matches) != 2 {
		t.Errorf("matches = %d, want 2", len(ambiguous.Matches))
	}
}

func TestResolveReportsNoMatchForUnknownTrackNumber(t *testing.T) {
	candidates := []Track{
		{ID: "a", Title: "Intro", ArtistName: "LiSA", AlbumName: "Launcher", TrackNumber: 1},
	}

	if _, err := Resolve("Intro", Criteria{Track: 99}, candidates); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("error = %v, want ErrNoMatch", err)
	}
}

func TestResolveReportsNoMatchForUnknownDisc(t *testing.T) {
	candidates := []Track{
		{ID: "a", Title: "Intro", ArtistName: "LiSA", AlbumName: "Launcher", DiscNumber: 1, TrackNumber: 1},
	}

	if _, err := Resolve("Intro", Criteria{Disc: 5}, candidates); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("error = %v, want ErrNoMatch", err)
	}
}

func TestResolveFuzzyTitle(t *testing.T) {
	candidates := []Track{
		{ID: "a", Title: "Haruka Mirai", ArtistName: "KANKAKU PIERO", AlbumName: "Iroirotoiro"},
	}

	track, err := Resolve("Harukamirai", Criteria{}, candidates)
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

	if _, err := Resolve("Harukamirai", Criteria{}, candidates); err == nil {
		t.Fatal("Resolve returned nil error, want an ambiguity refusal")
	}
}

func TestResolveReturnsNoMatchForUnrelatedTitle(t *testing.T) {
	candidates := []Track{
		{ID: "a", Title: "Disorder", ArtistName: "神崎エルザ"},
	}

	if _, err := Resolve("Completely Different", Criteria{}, candidates); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("error = %v, want ErrNoMatch", err)
	}
}

func TestResolveReturnsNoMatchForUnknownArtist(t *testing.T) {
	candidates := []Track{
		{ID: "a", Title: "Disorder", ArtistName: "神崎エルザ"},
	}

	if _, err := Resolve("Disorder", Criteria{Artist: "Nobody At All"}, candidates); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("error = %v, want ErrNoMatch", err)
	}
}

func TestResolveReturnsNoMatchForEmptyCandidates(t *testing.T) {
	if _, err := Resolve("Disorder", Criteria{}, nil); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("error = %v, want ErrNoMatch", err)
	}
}
