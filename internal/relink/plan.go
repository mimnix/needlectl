// Package relink links the entries of an existing playlist that cannot be
// played yet to files of the native DroppedNeedle library.
package relink

import (
	"github.com/mimnix/needlectl/internal/api"
	"github.com/mimnix/needlectl/internal/link"
	"github.com/mimnix/needlectl/internal/matcher"
)

// Relink describes one playlist entry that can be linked to a library track.
type Relink struct {
	Existing api.PlaylistTrack
	Library  matcher.Track
	Track    api.TrackInput
}

// Plan is the pure result of scanning a playlist against the library.
type Plan struct {
	Total     int
	Playable  int
	Unmatched int
	Relinks   []Relink
}

// Changed reports whether the plan would modify the playlist.
func (p Plan) Changed() bool { return len(p.Relinks) > 0 }

// BuildPlan finds the entries of the playlist that cannot be played yet and
// matches them against the library.
//
// Entries that already have a source are counted as playable and left
// untouched, so a working link is never replaced by a library match.
func BuildPlan(tracks []api.PlaylistTrack, index *matcher.Index) Plan {
	plan := Plan{Total: len(tracks)}

	for _, track := range tracks {
		if track.Linked() {
			plan.Playable++
			continue
		}
		library, matched := index.Match(track.TrackName, track.ArtistName, track.AlbumName)
		if !matched {
			plan.Unmatched++
			continue
		}
		plan.Relinks = append(plan.Relinks, Relink{
			Existing: track,
			Library:  library,
			Track:    link.Input(track.TrackName, track.ArtistName, track.AlbumName, library),
		})
	}
	return plan
}
