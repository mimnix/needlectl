package albums

import (
	"errors"
	"testing"
)

func TestResolveReturnsUniqueExactTitle(t *testing.T) {
	candidates := []Album{
		{ID: "a", Title: "Elza", ArtistName: "神崎エルザ"},
		{ID: "b", Title: "Launcher", ArtistName: "LiSA"},
	}

	album, err := Resolve("elza", "", candidates)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if album.ID != "a" {
		t.Errorf("resolved %q, want a", album.ID)
	}
}

func TestResolveReportsAmbiguousExactTitles(t *testing.T) {
	candidates := []Album{
		{ID: "a", Title: "Elza", ArtistName: "神崎エルザ"},
		{ID: "b", Title: "Elza", ArtistName: "Someone Else"},
	}

	_, err := Resolve("Elza", "", candidates)

	var ambiguous *AmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("error = %v, want *AmbiguousError", err)
	}
	if len(ambiguous.Matches) != 2 {
		t.Errorf("matches = %d, want 2", len(ambiguous.Matches))
	}
}

func TestResolveArtistNarrowsAmbiguity(t *testing.T) {
	candidates := []Album{
		{ID: "a", Title: "Elza", ArtistName: "神崎エルザ"},
		{ID: "b", Title: "Elza", ArtistName: "Someone Else"},
	}

	album, err := Resolve("Elza", "Someone Else", candidates)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if album.ID != "b" {
		t.Errorf("resolved %q, want b", album.ID)
	}
}

func TestResolveFuzzyTitle(t *testing.T) {
	candidates := []Album{
		{ID: "a", Title: "Launcher", ArtistName: "LiSA"},
	}

	album, err := Resolve("Launcer", "", candidates)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if album.ID != "a" {
		t.Errorf("resolved %q, want a", album.ID)
	}
}

func TestResolveReturnsNoMatchForUnrelatedTitle(t *testing.T) {
	candidates := []Album{
		{ID: "a", Title: "Launcher", ArtistName: "LiSA"},
	}

	_, err := Resolve("Completely Different Album", "", candidates)
	if !errors.Is(err, ErrNoMatch) {
		t.Fatalf("error = %v, want ErrNoMatch", err)
	}
}

func TestResolveReturnsNoMatchForUnknownArtist(t *testing.T) {
	candidates := []Album{
		{ID: "a", Title: "Elza", ArtistName: "神崎エルザ"},
	}

	_, err := Resolve("Elza", "Nobody At All", candidates)
	if !errors.Is(err, ErrNoMatch) {
		t.Fatalf("error = %v, want ErrNoMatch", err)
	}
}

func TestResolveReturnsNoMatchForEmptyCandidates(t *testing.T) {
	_, err := Resolve("Elza", "", nil)
	if !errors.Is(err, ErrNoMatch) {
		t.Fatalf("error = %v, want ErrNoMatch", err)
	}
}

func TestResolveRefusesToGuessOnEqualScores(t *testing.T) {
	candidates := []Album{
		{ID: "a", Title: "Launcher", ArtistName: "LiSA"},
		{ID: "b", Title: "Launcher", ArtistName: "LiSA"},
	}

	if _, err := Resolve("Launcher", "", candidates); err == nil {
		t.Fatal("Resolve returned nil error, want an ambiguity refusal")
	}
}
