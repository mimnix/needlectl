// Package link holds the primitives shared by the commands that tie playlist
// entries to files of the native DroppedNeedle library.
package link

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/mimnix/needlectl/internal/api"
	"github.com/mimnix/needlectl/internal/matcher"
)

// SourceTypeLocal is the source_type the API expects for a native library file.
const SourceTypeLocal = "local"

// coverSize is the artwork rendition DroppedNeedle stores on playlist entries.
const coverSize = "250"

// Index builds a matcher index over the native library.
func Index(tracks []api.LibraryTrack) *matcher.Index {
	converted := make([]matcher.Track, len(tracks))
	for i, track := range tracks {
		converted[i] = matcher.Track{
			ID:              track.ID,
			Title:           track.Title,
			AlbumID:         track.AlbumID,
			ReleaseGroupID:  derefString(track.ReleaseGroupMBID),
			AlbumTitle:      track.AlbumTitle,
			ArtistID:        track.ArtistID,
			ArtistName:      track.ArtistName,
			DiscNumber:      track.DiscNumber,
			TrackNumber:     track.TrackNumber,
			DurationSeconds: track.DurationSeconds,
			Format:          track.Format,
			CoverAvailable:  track.CoverAvailable,
		}
	}
	return matcher.NewIndex(converted)
}

// CoverURL returns the DroppedNeedle artwork URL for the album of a library
// track, or "" when the track has no release group.
//
// Playlist entries store the artwork reference themselves and the server never
// derives one from the library file, so an entry added without this URL shows no
// cover.
//
// The covers endpoint takes the MusicBrainz release-group id, not the local
// album id: in v3 a library track's AlbumID is the local album and its
// ReleaseGroupMBID is the id to link. CoverAvailable is not a reliable gate
// either — it reflects locally scanned art, while the cover endpoint also
// fetches art for the release group, so it is true for albums that would show a
// cover.
func CoverURL(library matcher.Track) string {
	return CoverURLForReleaseGroup(library.ReleaseGroupID)
}

// CoverURLForReleaseGroup builds the artwork URL for one MusicBrainz release
// group, or returns "" when the id is empty.
func CoverURLForReleaseGroup(releaseGroupID string) string {
	trimmed := stringValue(releaseGroupID)
	if trimmed == "" {
		return ""
	}
	return "/api/v3/covers/release-group/" + url.PathEscape(trimmed) + "?size=" + coverSize
}

// Input builds the add-tracks payload that links a playlist entry to library.
// The entry keeps its own title/artist/album so the playlist does not change
// when the library spells them differently.
func Input(title, artist, album string, library matcher.Track) api.TrackInput {
	input := api.TrackInput{
		TrackName:        title,
		ArtistName:       artist,
		AlbumName:        album,
		AlbumID:          stringPointer(library.AlbumID),
		ArtistID:         stringPointer(library.ArtistID),
		TrackSourceID:    stringPointer(library.ID),
		CoverURL:         stringPointer(CoverURL(library)),
		SourceType:       SourceTypeLocal,
		AvailableSources: []string{SourceTypeLocal},
		Format:           stringPointer(library.Format),
		TrackNumber:      positivePointer(library.TrackNumber),
		DiscNumber:       positivePointer(library.DiscNumber),
	}
	if library.DurationSeconds > 0 {
		input.Duration = intPointer(int(library.DurationSeconds + 0.5))
	}
	return input
}

// Replacer is the part of the API client that Replace needs.
type Replacer interface {
	AddTracks(ctx context.Context, playlistID string, tracks []api.TrackInput, position *int) ([]api.PlaylistTrack, error)
	RemoveTracks(ctx context.Context, playlistID string, trackIDs []string) error
}

// Replace links an existing playlist entry to a library track in place.
//
// The API cannot attach a library file to an entry that already exists: PATCH
// only accepts source_type/available_sources and resolves the file from stored
// album metadata. The linked replacement is therefore inserted at the old
// position and the old entry removed afterwards. Inserting first means a failed
// insert leaves the original entry untouched, while a failed removal leaves a
// harmless duplicate instead of a hole.
func Replace(ctx context.Context, client Replacer, playlistID string, existing api.PlaylistTrack, input api.TrackInput) error {
	position := existing.Position
	if _, err := client.AddTracks(ctx, playlistID, []api.TrackInput{input}, &position); err != nil {
		return fmt.Errorf("add linked entry: %w", err)
	}
	if err := client.RemoveTracks(ctx, playlistID, []string{existing.ID}); err != nil {
		return fmt.Errorf("remove replaced entry (a duplicate may remain): %w", err)
	}
	return nil
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringValue(value string) string {
	return strings.TrimSpace(value)
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func intPointer(value int) *int { return &value }

func positivePointer(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}
