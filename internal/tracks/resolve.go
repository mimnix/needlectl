// Package tracks resolves a user-supplied track query against the tracks of the
// DroppedNeedle library. Like the albums resolver, it returns a track only when
// exactly one candidate clearly matches, and otherwise reports the ambiguity so
// a destructive command never targets the wrong track.
package tracks

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mimmo/needlectl/internal/matcher"
)

// Track is a library track in the shape the resolver needs.
type Track struct {
	ID              string
	Title           string
	ArtistName      string
	AlbumName       string
	TrackNumber     int
	DiscNumber      int
	DurationSeconds float64
	Format          string
}

// ErrNoMatch is returned when no track matches the query.
var ErrNoMatch = errors.New("no track matched")

// AmbiguousError is returned when several tracks match the query equally well.
type AmbiguousError struct {
	Query   string
	Matches []Track
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("%q matches %d tracks", e.Query, len(e.Matches))
}

const (
	// A fuzzy candidate must clear this title similarity before scoring.
	minTitleSimilarity = 0.75
	// The weighted score a fuzzy candidate must reach to be a match.
	minTrackScore = 0.80
	// Candidates within this score of the best are considered a tie.
	ambiguityDelta = 0.03
)

// Resolve finds the single track best matching query, optionally restricted to
// artist and/or album. It returns ErrNoMatch when nothing is close enough and
// *AmbiguousError when several candidates tie.
func Resolve(query, artist, album string, candidates []Track) (Track, error) {
	if len(candidates) == 0 {
		return Track{}, fmt.Errorf("%w: %q", ErrNoMatch, query)
	}

	filtered := candidates
	if strings.TrimSpace(artist) != "" {
		if filtered = filterByArtist(filtered, artist); len(filtered) == 0 {
			return Track{}, fmt.Errorf("%w: %q by %q", ErrNoMatch, query, artist)
		}
	}
	if strings.TrimSpace(album) != "" {
		if filtered = filterByAlbum(filtered, album); len(filtered) == 0 {
			return Track{}, fmt.Errorf("%w: %q on %q", ErrNoMatch, query, album)
		}
	}

	normalizedQuery := matcher.Normalize(query)
	var exact []Track
	for _, candidate := range filtered {
		if matcher.Normalize(candidate.Title) == normalizedQuery {
			exact = append(exact, candidate)
		}
	}
	switch len(exact) {
	case 1:
		return exact[0], nil
	case 0:
		// fall through to fuzzy matching
	default:
		return Track{}, &AmbiguousError{Query: query, Matches: exact}
	}

	scored := scoreCandidates(normalizedQuery, artist, album, filtered)
	if len(scored) == 0 {
		return Track{}, fmt.Errorf("%w: %q", ErrNoMatch, query)
	}
	best := scored[0]
	if best.Score < minTrackScore {
		return Track{}, fmt.Errorf("%w: %q", ErrNoMatch, query)
	}
	if len(scored) > 1 && best.Score-scored[1].Score <= ambiguityDelta {
		return Track{}, &AmbiguousError{Query: query, Matches: tiedMatches(scored, best.Score)}
	}
	return best.Track, nil
}

type scoredTrack struct {
	Track Track
	Score float64
}

// scoreCandidates weights the requested fields only, so the score does not
// depend on metadata the user did not provide.
func scoreCandidates(normalizedQuery, artist, album string, candidates []Track) []scoredTrack {
	normalizedArtist := matcher.Normalize(artist)
	normalizedAlbum := matcher.Normalize(album)

	scored := make([]scoredTrack, 0, len(candidates))
	for _, candidate := range candidates {
		titleScore := matcher.Similarity(normalizedQuery, matcher.Normalize(candidate.Title))
		if titleScore < minTitleSimilarity {
			continue
		}

		weighted, totalWeight := titleScore*0.6, 0.6
		if artist != "" {
			weighted += matcher.Similarity(normalizedArtist, matcher.Normalize(candidate.ArtistName)) * 0.25
			totalWeight += 0.25
		}
		if album != "" {
			weighted += matcher.Similarity(normalizedAlbum, matcher.Normalize(candidate.AlbumName)) * 0.15
			totalWeight += 0.15
		}
		scored = append(scored, scoredTrack{Track: candidate, Score: weighted / totalWeight})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].Track.ID < scored[j].Track.ID
	})
	return scored
}

func tiedMatches(scored []scoredTrack, best float64) []Track {
	var matches []Track
	for _, candidate := range scored {
		if best-candidate.Score <= ambiguityDelta {
			matches = append(matches, candidate.Track)
		}
	}
	return matches
}

// filterByArtist narrows candidates to the requested artist, preferring exact
// normalized matches and falling back to fuzzy ones.
func filterByArtist(candidates []Track, artist string) []Track {
	target := matcher.Normalize(artist)
	if exact := filter(candidates, func(c Track) bool {
		return matcher.Normalize(c.ArtistName) == target
	}); len(exact) > 0 {
		return exact
	}
	return filter(candidates, func(c Track) bool {
		return matcher.Similarity(target, matcher.Normalize(c.ArtistName)) >= 0.80
	})
}

// filterByAlbum narrows candidates to the requested album, preferring exact
// normalized matches and falling back to fuzzy ones.
func filterByAlbum(candidates []Track, album string) []Track {
	target := matcher.Normalize(album)
	if exact := filter(candidates, func(c Track) bool {
		return matcher.Normalize(c.AlbumName) == target
	}); len(exact) > 0 {
		return exact
	}
	return filter(candidates, func(c Track) bool {
		return matcher.Similarity(target, matcher.Normalize(c.AlbumName)) >= 0.75
	})
}

func filter(candidates []Track, keep func(Track) bool) []Track {
	var out []Track
	for _, candidate := range candidates {
		if keep(candidate) {
			out = append(out, candidate)
		}
	}
	return out
}
