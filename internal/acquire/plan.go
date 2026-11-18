// Package acquire turns the unplayable entries of a playlist into album
// acquisition requests.
//
// DroppedNeedle downloads at album level, so a missing track is fetched by
// requesting the release group that contains it. Playlist rows usually carry no
// MusicBrainz ids, so the album is first resolved through the server's search
// index and only accepted when both its title and artist match exactly.
package acquire

import (
	"strings"

	"github.com/mimmo/needlectl/internal/api"
	"github.com/mimmo/needlectl/internal/matcher"
	"github.com/mimmo/needlectl/internal/mbref"
)

// MissingAlbum groups the unplayable entries of a playlist that belong to one
// album.
type MissingAlbum struct {
	Artist  string
	Album   string
	Entries []api.PlaylistTrack
}

// MissingAlbums returns the albums behind the entries that have no source yet,
// in playlist order. Entries without an album name are kept as their own group
// with Album == "" so the caller can report them.
func MissingAlbums(entries []api.PlaylistTrack) []MissingAlbum {
	var result []MissingAlbum
	index := make(map[string]int)

	for _, entry := range entries {
		if entry.Linked() {
			continue
		}
		artist := strings.TrimSpace(entry.ArtistName)
		album := strings.TrimSpace(entry.AlbumName)
		key := matcher.Normalize(artist) + "\x00" + matcher.Normalize(album)

		if at, ok := index[key]; ok {
			result[at].Entries = append(result[at].Entries, entry)
			continue
		}
		index[key] = len(result)
		result = append(result, MissingAlbum{
			Artist:  artist,
			Album:   album,
			Entries: []api.PlaylistTrack{entry},
		})
	}

	return result
}

// Choice is the album a search result resolved to.
type Choice struct {
	MusicBrainzID string
	Title         string
	Artist        string
	Year          *int
	InLibrary     bool
	Requested     bool
	// Alternatives counts further exact hits the search returned after the
	// chosen one, so a duplicate release group is visible in the report.
	Alternatives int
}

// ChooseAlbum picks the release group whose title and artist both match the
// album exactly. The server ranks its hits best first, so the first exact match
// wins; anything less certain is refused rather than guessed.
func ChooseAlbum(results []api.SearchResultItem, artist, album string) (Choice, bool) {
	normalizedAlbum := matcher.Normalize(album)
	normalizedArtist := matcher.Normalize(artist)
	if normalizedAlbum == "" {
		return Choice{}, false
	}

	var exact, albums []api.SearchResultItem
	for _, hit := range results {
		if !strings.EqualFold(hit.Kind, "album") {
			continue
		}
		if hit.MusicBrainzID == nil || strings.TrimSpace(*hit.MusicBrainzID) == "" {
			continue
		}
		if matcher.Normalize(hit.Title) != normalizedAlbum {
			continue
		}
		if hit.Artist == nil || matcher.Normalize(*hit.Artist) != normalizedArtist {
			continue
		}
		exact = append(exact, hit)
		if isAlbumType(hit.TypeInfo) {
			albums = append(albums, hit)
		}
	}
	if len(exact) == 0 {
		return Choice{}, false
	}

	// An album and its lead single often share title and artist. The playlist
	// entry belongs to the album, and the single's short tracklist would leave
	// most of the entries unmatched, so album hits win.
	candidates := exact
	if len(albums) > 0 {
		candidates = albums
	}
	top := candidates[0]
	return Choice{
		MusicBrainzID: strings.TrimSpace(*top.MusicBrainzID),
		Title:         top.Title,
		Artist:        deref(top.Artist),
		Year:          top.Year,
		InLibrary:     top.InLibrary,
		Requested:     top.Requested,
		Alternatives:  len(candidates) - 1,
	}, true
}

// ChooseRecording picks the release track whose title matches the playlist
// entry exactly. Anything else is refused: a free-text search can return a live
// version, and the album's own tracklist is the reliable source.
func ChooseRecording(tracks []mbref.ReleaseTrack, title string) (mbref.ReleaseTrack, bool) {
	normalized := matcher.Normalize(title)
	if normalized == "" {
		return mbref.ReleaseTrack{}, false
	}
	for _, track := range tracks {
		if matcher.Normalize(track.Title) == normalized {
			return track, true
		}
	}
	return mbref.ReleaseTrack{}, false
}

// isAlbumType reports whether a search hit is a MusicBrainz album rather than a
// single or EP.
func isAlbumType(info *string) bool {
	if info == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(*info), "Album")
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
