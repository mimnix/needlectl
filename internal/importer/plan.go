package importer

import (
	"github.com/mimmo/needlectl/internal/api"
	"github.com/mimmo/needlectl/internal/matcher"
	"github.com/mimmo/needlectl/internal/playlist"
)

const (
	sourceTypeLocal = "local"
	keySeparator    = "\x1f"
)

// Add describes a track that does not exist in the playlist yet.
type Add struct {
	Row    playlist.Row
	Track  api.TrackInput
	Linked bool
}

// Refresh describes an existing unlinked entry that is now available in the
// library and can therefore be linked.
type Refresh struct {
	Row      playlist.Row
	Existing api.PlaylistTrack
	Track    api.TrackInput
}

// Plan is the pure result of diffing the CSV rows against the playlist state.
type Plan struct {
	TotalRows int
	Matched   int
	Unmatched int
	Unchanged int
	Adds      []Add
	Refreshes []Refresh
}

// Changed reports whether the plan would modify the playlist.
func (p Plan) Changed() bool { return len(p.Adds) > 0 || len(p.Refreshes) > 0 }

// BuildPlan matches every CSV row against the library index and diffs it
// against the playlist's current entries. The diff is a multiset comparison, so
// legitimate duplicates in the CSV are preserved.
func BuildPlan(rows []playlist.Row, existing []api.PlaylistTrack, index *matcher.Index) Plan {
	plan := Plan{TotalRows: len(rows)}

	queues := make(map[string][]api.PlaylistTrack)
	for _, track := range existing {
		key := entryKey(track.TrackName, track.ArtistName, track.AlbumName)
		queues[key] = append(queues[key], track)
	}

	for _, row := range rows {
		libraryTrack, matched := index.Match(row.TrackName, row.ArtistName, row.AlbumName)
		if matched {
			plan.Matched++
		} else {
			plan.Unmatched++
		}

		key := entryKey(row.TrackName, row.ArtistName, row.AlbumName)
		if queue := queues[key]; len(queue) > 0 {
			current := queue[0]
			queues[key] = queue[1:]

			if !current.Linked() && matched {
				plan.Refreshes = append(plan.Refreshes, Refresh{
					Row:      row,
					Existing: current,
					Track:    buildTrackInput(row, libraryTrack, true),
				})
				continue
			}
			plan.Unchanged++
			continue
		}

		plan.Adds = append(plan.Adds, Add{
			Row:    row,
			Track:  buildTrackInput(row, libraryTrack, matched),
			Linked: matched,
		})
	}
	return plan
}

// buildTrackInput converts a CSV row (optionally enriched with a library match)
// into the API request shape.
func buildTrackInput(row playlist.Row, library matcher.Track, linked bool) api.TrackInput {
	input := api.TrackInput{
		TrackName:  row.TrackName,
		ArtistName: row.ArtistName,
		AlbumName:  row.AlbumName,
	}
	if !linked {
		if row.DurationMs != nil {
			input.Duration = intPointer(*row.DurationMs / 1000)
		}
		return input
	}

	input.AlbumID = stringPointer(library.AlbumID)
	input.ArtistID = stringPointer(library.ArtistID)
	input.TrackSourceID = stringPointer(library.ID)
	input.SourceType = sourceTypeLocal
	input.AvailableSources = []string{sourceTypeLocal}
	input.Format = stringPointer(library.Format)
	input.TrackNumber = positivePointer(library.TrackNumber)
	input.DiscNumber = positivePointer(library.DiscNumber)
	if library.DurationSeconds > 0 {
		input.Duration = intPointer(int(library.DurationSeconds + 0.5))
	}
	return input
}

func entryKey(track, artist, album string) string {
	return matcher.Normalize(track) + keySeparator +
		matcher.Normalize(artist) + keySeparator +
		matcher.Normalize(album)
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
