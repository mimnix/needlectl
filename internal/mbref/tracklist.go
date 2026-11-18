package mbref

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// ReleaseTrack is one recording of a release's tracklist.
type ReleaseTrack struct {
	RecordingMBID string
	Title         string
	Artist        string
	DiscNumber    int
	TrackNumber   int
	DurationMS    int
}

// Seconds returns the track length rounded to whole seconds, or nil when
// MusicBrainz did not report one.
func (t ReleaseTrack) Seconds() *int {
	if t.DurationMS <= 0 {
		return nil
	}
	seconds := (t.DurationMS + 500) / 1000
	return &seconds
}

// maxTracklistReleases bounds how many releases of a group are tried. The first
// official release is normally enough; a couple more cover groups whose first
// release has no files.
const maxTracklistReleases = 3

// ReleaseGroupTracks returns the tracklist of a release group's first usable
// release. It is what lets the CLI turn a playlist title into a recording MBID
// without the free-text recording search, which cannot tell a studio recording
// from a live one.
func (r *Resolver) ReleaseGroupTracks(ctx context.Context, releaseGroupMBID string) ([]ReleaseTrack, error) {
	trimmed := strings.ToLower(strings.TrimSpace(releaseGroupMBID))
	if !mbidPattern.MatchString(trimmed) {
		return nil, fmt.Errorf("%q is not a MusicBrainz release-group id", releaseGroupMBID)
	}

	releases, err := r.releasesOfGroup(ctx, trimmed)
	if err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("MusicBrainz release group %s has no releases", trimmed)
	}

	for i, release := range releases {
		if i >= maxTracklistReleases {
			break
		}
		tracks, err := r.releaseTracks(ctx, release)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		if len(tracks) > 0 {
			return tracks, nil
		}
	}
	return nil, fmt.Errorf("MusicBrainz has no tracklist for release group %s", trimmed)
}

type mbRelease struct {
	ID     string
	Date   string
	Status string
}

// releasesOfGroup lists the releases of a release group, official ones first
// and the earliest edition before reissues.
func (r *Resolver) releasesOfGroup(ctx context.Context, releaseGroupMBID string) ([]mbRelease, error) {
	query := url.Values{"inc": {"releases"}, "fmt": {"json"}}
	data, err := r.get(ctx, "/ws/2/release-group/"+url.PathEscape(releaseGroupMBID)+"?"+query.Encode())
	if err != nil {
		return nil, err
	}

	var payload struct {
		Releases []struct {
			ID     string `json:"id"`
			Date   string `json:"date"`
			Status string `json:"status"`
		} `json:"releases"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decode MusicBrainz release group %s: %w", releaseGroupMBID, err)
	}

	releases := make([]mbRelease, 0, len(payload.Releases))
	for _, release := range payload.Releases {
		if !mbidPattern.MatchString(release.ID) {
			continue
		}
		releases = append(releases, mbRelease{ID: release.ID, Date: release.Date, Status: release.Status})
	}
	sort.SliceStable(releases, func(i, j int) bool {
		iOfficial := strings.EqualFold(releases[i].Status, "Official")
		jOfficial := strings.EqualFold(releases[j].Status, "Official")
		if iOfficial != jOfficial {
			return iOfficial
		}
		return releases[i].Date < releases[j].Date
	})
	return releases, nil
}

type mbArtistCredit struct {
	Artist struct {
		Name string `json:"name"`
	} `json:"artist"`
}

// releaseTracks reads one release's tracklist.
func (r *Resolver) releaseTracks(ctx context.Context, release mbRelease) ([]ReleaseTrack, error) {
	query := url.Values{"inc": {"recordings+artist-credits"}, "fmt": {"json"}}
	data, err := r.get(ctx, "/ws/2/release/"+url.PathEscape(release.ID)+"?"+query.Encode())
	if err != nil {
		return nil, err
	}

	var payload struct {
		Media []struct {
			Position int `json:"position"`
			Tracks   []struct {
				Position  int    `json:"position"`
				Title     string `json:"title"`
				Length    int    `json:"length"`
				Recording struct {
					ID string `json:"id"`
				} `json:"recording"`
				ArtistCredit []mbArtistCredit `json:"artist-credit"`
			} `json:"tracks"`
		} `json:"media"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decode MusicBrainz release %s: %w", release.ID, err)
	}

	var tracks []ReleaseTrack
	for _, medium := range payload.Media {
		for _, track := range medium.Tracks {
			if !mbidPattern.MatchString(track.Recording.ID) {
				continue
			}
			tracks = append(tracks, ReleaseTrack{
				RecordingMBID: strings.ToLower(track.Recording.ID),
				Title:         strings.TrimSpace(track.Title),
				Artist:        creditNames(track.ArtistCredit),
				DiscNumber:    medium.Position,
				TrackNumber:   track.Position,
				DurationMS:    track.Length,
			})
		}
	}
	return tracks, nil
}

// creditNames joins an artist credit into one display string.
func creditNames(credit []mbArtistCredit) string {
	names := make([]string, 0, len(credit))
	for _, entry := range credit {
		if name := strings.TrimSpace(entry.Artist.Name); name != "" {
			names = append(names, name)
		}
	}
	return strings.Join(names, ", ")
}
