// Package albums resolves a user-supplied album query against the albums of the
// DroppedNeedle library. Resolution is deliberately conservative: it returns an
// album only when exactly one candidate is clearly the best, and otherwise
// reports an ambiguity so the caller never deletes the wrong album.
package albums

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mimnix/needlectl/internal/matcher"
)

// Album is a library album in the shape the resolver needs.
type Album struct {
	ID         string
	Title      string
	ArtistName string
	TrackCount int
	Format     string
	Year       int
}

// ErrNoMatch is returned when no album matches the query.
var ErrNoMatch = errors.New("no album matched")

// AmbiguousError is returned when several albums match the query equally well.
type AmbiguousError struct {
	Query   string
	Matches []Album
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("%q matches %d albums", e.Query, len(e.Matches))
}

// Minimum title similarity for a fuzzy (non-exact) match.
const minAlbumScore = 0.75

// Two candidates closer than this are considered a tie.
const ambiguityDelta = 0.05

// Resolve finds the single album best matching query, optionally restricted to
// artist. It returns ErrNoMatch when nothing is close enough and
// *AmbiguousError when several candidates tie.
func Resolve(query, artist string, candidates []Album) (Album, error) {
	if len(candidates) == 0 {
		return Album{}, fmt.Errorf("%w: %q", ErrNoMatch, query)
	}

	filtered := filterByArtist(candidates, artist)
	if len(filtered) == 0 {
		return Album{}, fmt.Errorf("%w: %q", ErrNoMatch, query)
	}

	normalizedQuery := matcher.Normalize(query)
	var exact []Album
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
		return Album{}, &AmbiguousError{Query: query, Matches: exact}
	}

	scored := scoreCandidates(normalizedQuery, artist, filtered)
	best := scored[0]
	if best.Score < minAlbumScore {
		return Album{}, fmt.Errorf("%w: %q", ErrNoMatch, query)
	}
	if len(scored) > 1 && best.Score-scored[1].Score <= ambiguityDelta {
		return Album{}, &AmbiguousError{Query: query, Matches: tiedMatches(scored, best.Score)}
	}
	return best.Album, nil
}

type scoredAlbum struct {
	Album Album
	Score float64
}

func scoreCandidates(normalizedQuery, artist string, candidates []Album) []scoredAlbum {
	normalizedArtist := matcher.Normalize(artist)
	scored := make([]scoredAlbum, 0, len(candidates))
	for _, candidate := range candidates {
		titleScore := matcher.Similarity(normalizedQuery, matcher.Normalize(candidate.Title))
		if artist != "" {
			artistScore := matcher.Similarity(normalizedArtist, matcher.Normalize(candidate.ArtistName))
			titleScore = 0.75*titleScore + 0.25*artistScore
		}
		scored = append(scored, scoredAlbum{Album: candidate, Score: titleScore})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].Album.ID < scored[j].Album.ID
	})
	return scored
}

func tiedMatches(scored []scoredAlbum, best float64) []Album {
	var matches []Album
	for _, candidate := range scored {
		if best-candidate.Score <= ambiguityDelta {
			matches = append(matches, candidate.Album)
		}
	}
	return matches
}

// filterByArtist narrows candidates to the requested artist, preferring exact
// normalized matches and falling back to fuzzy ones.
func filterByArtist(candidates []Album, artist string) []Album {
	if strings.TrimSpace(artist) == "" {
		return candidates
	}

	target := matcher.Normalize(artist)
	exact := make([]Album, 0, len(candidates))
	for _, candidate := range candidates {
		if matcher.Normalize(candidate.ArtistName) == target {
			exact = append(exact, candidate)
		}
	}
	if len(exact) > 0 {
		return exact
	}

	fuzzy := make([]Album, 0, len(candidates))
	for _, candidate := range candidates {
		if matcher.Similarity(target, matcher.Normalize(candidate.ArtistName)) >= 0.80 {
			fuzzy = append(fuzzy, candidate)
		}
	}
	return fuzzy
}
