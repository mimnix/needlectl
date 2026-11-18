// Package covers repairs the album artwork references stored on playlist
// entries.
//
// DroppedNeedle stores a cover URL on each playlist entry and never derives one
// from the linked library file, so an entry added before the artwork existed (or
// with the wrong id) keeps showing no cover. The API cannot patch that field:
// only source_type and available_sources are writable, so a repair replaces the
// entry at its position, like linking does.
package covers

import (
	"strings"

	"github.com/mimnix/needlectl/internal/api"
	"github.com/mimnix/needlectl/internal/link"
)

// Repair is one playlist entry whose stored cover URL is wrong or missing.
type Repair struct {
	// Existing is the entry to replace.
	Existing api.PlaylistTrack
	// Input is the replacement, identical except for the cover URL.
	Input api.TrackInput
	// Old is the stored cover URL (empty when the entry had none).
	Old string
	// New is the cover URL the linked album should carry.
	New string
}

// Plan is what repairing one playlist's covers would change.
type Plan struct {
	Total          int
	Repairs        []Repair
	Unchanged      int
	NoLibrary      int
	NoReleaseGroup int
}

// Index maps library track ids to the tracks themselves.
func Index(tracks []api.LibraryTrack) map[string]api.LibraryTrack {
	index := make(map[string]api.LibraryTrack, len(tracks))
	for _, track := range tracks {
		index[track.ID] = track
	}
	return index
}

// BuildPlan compares every entry with the cover URL its linked album should
// carry. Entries that are not linked to the local library, or whose album has no
// MusicBrainz release group, are counted and left alone.
func BuildPlan(entries []api.PlaylistTrack, library map[string]api.LibraryTrack) Plan {
	plan := Plan{Total: len(entries)}

	for _, entry := range entries {
		track, ok := linkedTrack(entry, library)
		if !ok {
			plan.NoLibrary++
			continue
		}

		desired := link.CoverURLForReleaseGroup(deref(track.ReleaseGroupMBID))
		if desired == "" {
			plan.NoReleaseGroup++
			continue
		}

		stored := deref(entry.CoverURL)
		if stored == desired {
			plan.Unchanged++
			continue
		}

		plan.Repairs = append(plan.Repairs, Repair{
			Existing: entry,
			Input:    repairInput(entry, desired),
			Old:      stored,
			New:      desired,
		})
	}

	return plan
}

// linkedTrack returns the library track a playlist entry points at. The entry
// carries the library track id in track_source_id; older entries only carry
// library_file_id, which holds the same id in practice.
func linkedTrack(entry api.PlaylistTrack, library map[string]api.LibraryTrack) (api.LibraryTrack, bool) {
	for _, id := range []*string{entry.TrackSourceID, entry.LibraryFileID} {
		if id == nil {
			continue
		}
		trimmed := strings.TrimSpace(*id)
		if trimmed == "" {
			continue
		}
		if track, ok := library[trimmed]; ok {
			return track, true
		}
	}
	return api.LibraryTrack{}, false
}

// repairInput rebuilds the add-track payload for an entry, preserving every
// stored field and changing only the cover URL.
func repairInput(entry api.PlaylistTrack, coverURL string) api.TrackInput {
	input := api.TrackInput{
		TrackName:        entry.TrackName,
		ArtistName:       entry.ArtistName,
		AlbumName:        entry.AlbumName,
		AlbumID:          entry.AlbumID,
		ArtistID:         entry.ArtistID,
		TrackSourceID:    entry.TrackSourceID,
		CoverURL:         &coverURL,
		SourceType:       entry.SourceType,
		AvailableSources: entry.AvailableSources,
		Format:           entry.Format,
		TrackNumber:      entry.TrackNumber,
		DiscNumber:       entry.DiscNumber,
		RecordingMBID:    entry.RecordingMBID,
		PlexRatingKey:    entry.PlexRatingKey,
	}
	if input.TrackSourceID == nil && entry.LibraryFileID != nil {
		input.TrackSourceID = entry.LibraryFileID
	}
	if strings.TrimSpace(input.SourceType) == "" {
		input.SourceType = link.SourceTypeLocal
	}
	if len(input.AvailableSources) == 0 {
		input.AvailableSources = []string{link.SourceTypeLocal}
	}
	if entry.Duration != nil && *entry.Duration > 0 {
		seconds := int(*entry.Duration + 0.5)
		input.Duration = &seconds
	}
	return input
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
