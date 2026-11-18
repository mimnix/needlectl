package acquire

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mimnix/needlectl/internal/api"
	"github.com/mimnix/needlectl/internal/mbref"
	"github.com/mimnix/needlectl/internal/output"
)

// maxListedAlbums caps how many albums the report prints unless verbose output
// is on.
const maxListedAlbums = 20

// Mode selects what is requested for a missing track.
type Mode int

const (
	// ModeTrack requests each missing recording on its own.
	ModeTrack Mode = iota
	// ModeAlbum requests the release group containing the missing tracks once.
	ModeAlbum
)

// String returns the mode as the command reports it.
func (m Mode) String() string {
	if m == ModeAlbum {
		return "per album"
	}
	return "per track"
}

// API is the subset of the DroppedNeedle client the acquisition needs.
type API interface {
	ListPlaylists(ctx context.Context) ([]api.PlaylistSummary, error)
	GetPlaylist(ctx context.Context, id string) (*api.PlaylistDetail, error)
	SearchAlbums(ctx context.Context, query string, limit int) ([]api.SearchResultItem, error)
	RequestAlbum(ctx context.Context, request api.AlbumRequest) (*api.IntakeResponse, error)
	RequestTrack(ctx context.Context, request api.TrackRequest) (*api.TrackIntakeResponse, error)
}

// TrackResolver returns the tracklist of a MusicBrainz release group.
type TrackResolver interface {
	ReleaseGroupTracks(ctx context.Context, releaseGroupMBID string) ([]mbref.ReleaseTrack, error)
}

// Target identifies the playlist whose missing tracks are requested.
type Target struct {
	ID   string
	Name string
}

// AmbiguousPlaylistError reports that a playlist name matched several playlists.
type AmbiguousPlaylistError struct {
	Name    string
	Matches []api.PlaylistSummary
}

func (e *AmbiguousPlaylistError) Error() string {
	return fmt.Sprintf("%q matches %d playlists; pass the playlist id with --id", e.Name, len(e.Matches))
}

// AlbumResult is the outcome for one missing album.
type AlbumResult struct {
	Missing MissingAlbum
	Choice  Choice
	// Status is the server status in album mode, or one of "requested",
	// "partial", "would request", "already requested", "unmatched" and "failed".
	Status          string
	Message         string
	Requested       int
	Unmatched       int
	Failed          int
	UnmatchedTitles []string
}

// Result summarizes one playlist's acquisition.
type Result struct {
	PlaylistID   string
	PlaylistName string
	Mode         Mode
	Total        int
	Missing      int
	Albums       int
	Requested    int
	AlreadyAsked int
	Unmatched    int
	Failed       int
	Covered      int
	Items        []AlbumResult
}

// Runner requests the tracks or albums behind a playlist's unplayable entries.
type Runner struct {
	API    API
	Tracks TrackResolver
	Out    *output.Printer
	DryRun bool
	Mode   Mode
	// Pause is slept between albums so a bulk run stays under the server's
	// request limit. Zero disables it (tests).
	Pause time.Duration

	tracklists  map[string][]mbref.ReleaseTrack
	trackErrors map[string]error
}

// Run requests the missing tracks of target.
func (r *Runner) Run(ctx context.Context, target Target) (Result, error) {
	result := Result{PlaylistName: target.Name, Mode: r.Mode}

	id, err := r.playlistID(ctx, target)
	if err != nil {
		return result, err
	}
	detail, err := r.API.GetPlaylist(ctx, id)
	if err != nil {
		return result, fmt.Errorf("load playlist %s: %w", id, err)
	}
	result.PlaylistID = detail.ID
	result.PlaylistName = detail.Name
	result.Total = len(detail.Tracks)

	albums := MissingAlbums(detail.Tracks)
	for _, album := range albums {
		result.Missing += len(album.Entries)
	}
	result.Albums = len(albums)

	r.step("Checking the albums of %q", detail.Name)
	r.info("%d entries · %d not playable · %d albums · %s",
		result.Total, result.Missing, result.Albums, r.Mode)
	if result.Missing == 0 {
		return result, nil
	}

	for i, album := range albums {
		if i > 0 {
			if err := r.wait(ctx); err != nil {
				return result, err
			}
		}
		item := r.resolveAlbum(ctx, album)
		result.Items = append(result.Items, item)
		r.accumulate(&result, item)
	}

	r.printItems(result.Items)
	return result, nil
}

func (r *Runner) accumulate(result *Result, item AlbumResult) {
	result.Requested += item.Requested
	result.Unmatched += item.Unmatched
	result.Failed += item.Failed

	if item.Status == "already requested" {
		result.AlreadyAsked++
		result.Covered += len(item.Missing.Entries)
		return
	}
	if r.Mode == ModeAlbum {
		if item.Requested > 0 {
			result.Covered += len(item.Missing.Entries)
		}
		return
	}
	result.Covered += item.Requested
}

// resolveAlbum searches one album and requests its tracks or the album itself.
func (r *Runner) resolveAlbum(ctx context.Context, album MissingAlbum) AlbumResult {
	item := AlbumResult{Missing: album}
	if album.Album == "" {
		item.Status = "unmatched"
		item.Unmatched = len(album.Entries)
		item.Message = "the entries have no album name"
		return item
	}

	hits, err := r.API.SearchAlbums(ctx, album.Album+" "+album.Artist, 10)
	if err != nil {
		item.Status = "failed"
		item.Unmatched = len(album.Entries)
		item.Message = err.Error()
		return item
	}

	choice, ok := ChooseAlbum(hits, album.Artist, album.Album)
	if !ok {
		item.Status = "unmatched"
		item.Unmatched = len(album.Entries)
		item.Message = "no exact album and artist match"
		return item
	}
	item.Choice = choice

	if r.Mode == ModeAlbum {
		return r.requestAlbum(ctx, item)
	}
	return r.requestTracks(ctx, item)
}

func (r *Runner) requestAlbum(ctx context.Context, item AlbumResult) AlbumResult {
	if item.Choice.Requested {
		item.Status = "already requested"
		return item
	}
	if r.DryRun {
		item.Status = "would request"
		item.Requested = 1
		return item
	}

	request := api.AlbumRequest{
		MusicBrainzID: item.Choice.MusicBrainzID,
		Album:         stringPointer(item.Missing.Album),
		Artist:        stringPointer(item.Missing.Artist),
		Year:          item.Choice.Year,
	}
	response, err := r.API.RequestAlbum(ctx, request)
	if err != nil {
		item.Status = "failed"
		item.Message = err.Error()
		return item
	}
	if !response.Success {
		item.Status = "failed"
		item.Message = strings.TrimSpace(response.Message)
		return item
	}

	item.Requested = 1
	item.Status = strings.TrimSpace(response.Status)
	if item.Status == "" {
		item.Status = "queued"
	}
	item.Message = strings.TrimSpace(response.Message)
	return item
}

func (r *Runner) requestTracks(ctx context.Context, item AlbumResult) AlbumResult {
	if item.Choice.Requested {
		item.Status = "already requested"
		return item
	}
	if r.Tracks == nil {
		item.Status = "failed"
		item.Unmatched = len(item.Missing.Entries)
		item.Message = "no MusicBrainz tracklist resolver"
		return item
	}

	tracks, err := r.tracklist(ctx, item.Choice.MusicBrainzID)
	if err != nil {
		item.Status = "failed"
		item.Unmatched = len(item.Missing.Entries)
		item.Message = err.Error()
		return item
	}

	for _, entry := range item.Missing.Entries {
		recording, ok := ChooseRecording(tracks, entry.TrackName)
		if !ok {
			item.Unmatched++
			item.UnmatchedTitles = append(item.UnmatchedTitles, entry.TrackName)
			continue
		}
		if r.DryRun {
			item.Requested++
			continue
		}

		request := api.TrackRequest{
			RecordingMBID:    recording.RecordingMBID,
			ArtistName:       firstNonEmpty(entry.ArtistName, recording.Artist),
			TrackTitle:       firstNonEmpty(entry.TrackName, recording.Title),
			AlbumTitle:       stringPointer(item.Missing.Album),
			ReleaseGroupMBID: stringPointer(item.Choice.MusicBrainzID),
			DurationSeconds:  recording.Seconds(),
		}
		if _, err := r.API.RequestTrack(ctx, request); err != nil {
			item.Failed++
			item.Message = err.Error()
			continue
		}
		item.Requested++
	}

	total := len(item.Missing.Entries)
	switch {
	case item.Failed > 0 && item.Requested == 0:
		item.Status = "failed"
	case item.Failed > 0 || item.Unmatched > 0:
		item.Status = "partial"
	case r.DryRun:
		item.Status = "would request"
	case item.Requested == total:
		item.Status = "requested"
	default:
		item.Status = "unmatched"
	}
	return item
}

// tracklist resolves a release group's tracklist once per run.
func (r *Runner) tracklist(ctx context.Context, releaseGroupMBID string) ([]mbref.ReleaseTrack, error) {
	if r.tracklists == nil {
		r.tracklists = make(map[string][]mbref.ReleaseTrack)
		r.trackErrors = make(map[string]error)
	}
	if tracks, ok := r.tracklists[releaseGroupMBID]; ok {
		return tracks, nil
	}
	if err, ok := r.trackErrors[releaseGroupMBID]; ok {
		return nil, err
	}

	tracks, err := r.Tracks.ReleaseGroupTracks(ctx, releaseGroupMBID)
	if err != nil {
		r.trackErrors[releaseGroupMBID] = err
		return nil, err
	}
	r.tracklists[releaseGroupMBID] = tracks
	return tracks, nil
}

func (r *Runner) printItems(items []AlbumResult) {
	for i, item := range items {
		if i >= maxListedAlbums && !r.verbose() {
			r.info("… and %d more (use --verbose to list them all)", len(items)-i)
			return
		}
		r.info("%s %s — %s: %s", statusMark(item.Status), item.Missing.Album, item.Missing.Artist, r.itemLabel(item))
		if r.verbose() && r.Mode == ModeTrack {
			for _, title := range item.UnmatchedTitles {
				r.info("  %s — no matching recording", title)
			}
		}
	}
}

func (r *Runner) itemLabel(item AlbumResult) string {
	if r.Mode == ModeTrack {
		if item.Status == "already requested" {
			return "album already requested"
		}
		total := len(item.Missing.Entries)
		verb := "requested"
		if r.DryRun {
			verb = "to request"
		}
		label := fmt.Sprintf("%d/%d tracks %s", item.Requested, total, verb)
		if item.Unmatched > 0 {
			label += fmt.Sprintf(" · %d unmatched", item.Unmatched)
		}
		if item.Failed > 0 {
			label += fmt.Sprintf(" · %d failed", item.Failed)
		}
		if item.Message != "" {
			label += " · " + item.Message
		}
		return label
	}

	label := item.Status
	if item.Choice.Alternatives > 0 && (item.Status == "queued" || item.Status == "would request") {
		label += fmt.Sprintf(" (top of %d matches)", item.Choice.Alternatives+1)
	}
	if item.Message != "" {
		label += " · " + item.Message
	}
	return label
}

func statusMark(status string) string {
	switch status {
	case "unmatched", "failed":
		return "✗"
	case "partial":
		return "~"
	case "already requested":
		return "="
	case "would request":
		return "→"
	default:
		return "✓"
	}
}

func (r *Runner) playlistID(ctx context.Context, target Target) (string, error) {
	if strings.TrimSpace(target.ID) != "" {
		return strings.TrimSpace(target.ID), nil
	}

	name := strings.TrimSpace(target.Name)
	if name == "" {
		return "", fmt.Errorf("no playlist given")
	}

	summaries, err := r.API.ListPlaylists(ctx)
	if err != nil {
		return "", fmt.Errorf("list playlists: %w", err)
	}

	var matches []api.PlaylistSummary
	for _, summary := range summaries {
		candidate := strings.TrimSpace(summary.Name)
		if candidate == "" {
			continue // redacted playlist: no name visible
		}
		if strings.EqualFold(candidate, name) {
			matches = append(matches, summary)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("playlist %q not found", name)
	case 1:
		return matches[0].ID, nil
	default:
		return "", &AmbiguousPlaylistError{Name: name, Matches: matches}
	}
}

func (r *Runner) wait(ctx context.Context) error {
	if r.Pause <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(r.Pause):
		return nil
	}
}

func (r *Runner) step(format string, args ...any) {
	if r.Out != nil {
		r.Out.Step(format, args...)
	}
}

func (r *Runner) info(format string, args ...any) {
	if r.Out != nil {
		r.Out.Info(format, args...)
	}
}

func (r *Runner) verbose() bool {
	return r.Out != nil && r.Out.Verbose()
}

func stringPointer(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
