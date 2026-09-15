package importer

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mimmo/needlectl/internal/api"
	"github.com/mimmo/needlectl/internal/matcher"
	"github.com/mimmo/needlectl/internal/output"
	"github.com/mimmo/needlectl/internal/playlist"
)

const defaultBatchSize = 100

// API is the subset of the DroppedNeedle client the importer needs.
type API interface {
	ListLibraryTracks(ctx context.Context) ([]api.LibraryTrack, error)
	ListPlaylists(ctx context.Context) ([]api.PlaylistSummary, error)
	CreatePlaylist(ctx context.Context, name string) (*api.PlaylistDetail, error)
	GetPlaylist(ctx context.Context, id string) (*api.PlaylistDetail, error)
	AddTracks(ctx context.Context, playlistID string, tracks []api.TrackInput, position *int) ([]api.PlaylistTrack, error)
	RemoveTracks(ctx context.Context, playlistID string, trackIDs []string) error
	ResolveSources(ctx context.Context, playlistID string) (map[string][]string, error)
}

// Runner executes an import plan against the API.
type Runner struct {
	API            API
	Out            *output.Printer
	BatchSize      int
	ResolveSources bool
	DryRun         bool
}

// Result summarizes what an import run did (or would do).
type Result struct {
	PlaylistID      string
	PlaylistName    string
	PlaylistCreated bool
	TotalRows       int
	Matched         int
	Unmatched       int
	AddedLinked     int
	AddedUnlinked   int
	Refreshed       int
	Unchanged       int
	Playable        int
	StillUnlinked   int
	LibrarySize     int
}

// Run performs one idempotent import of rows into the named playlist.
func (r *Runner) Run(ctx context.Context, name string, rows []playlist.Row) (Result, error) {
	result := Result{PlaylistName: name, TotalRows: len(rows)}

	index, err := r.loadLibrary(ctx, &result)
	if err != nil {
		return result, err
	}

	playlistID, existing, err := r.ensurePlaylist(ctx, name, &result)
	if err != nil {
		return result, err
	}
	result.PlaylistID = playlistID

	plan := BuildPlan(rows, existing, index)
	result.Matched = plan.Matched
	result.Unmatched = plan.Unmatched
	result.Unchanged = plan.Unchanged

	r.printPlan(plan)
	if r.DryRun {
		result.AddedLinked = linkedCount(plan.Adds)
		result.AddedUnlinked = len(plan.Adds) - result.AddedLinked
		result.Refreshed = len(plan.Refreshes)
		return result, nil
	}

	if err := r.applyAdds(ctx, playlistID, plan, &result); err != nil {
		return result, err
	}
	if err := r.applyRefreshes(ctx, playlistID, plan, &result); err != nil {
		return result, err
	}
	if r.ResolveSources {
		r.resolveSources(ctx, playlistID)
	}
	r.countPlayable(ctx, playlistID, &result)
	return result, nil
}

func (r *Runner) loadLibrary(ctx context.Context, result *Result) (*matcher.Index, error) {
	r.step("Loading DroppedNeedle library")
	tracks, err := r.API.ListLibraryTracks(ctx)
	if err != nil {
		return nil, fmt.Errorf("load library: %w", err)
	}
	index := matcher.NewIndex(toMatcherTracks(tracks))
	result.LibrarySize = index.Len()
	r.info("%s indexed", countLabel(index.Len(), "track"))
	return index, nil
}

func (r *Runner) ensurePlaylist(ctx context.Context, name string, result *Result) (string, []api.PlaylistTrack, error) {
	r.step("Resolving playlist %q", name)
	summaries, err := r.API.ListPlaylists(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("list playlists: %w", err)
	}

	id := findPlaylistID(summaries, name)
	if id == "" {
		if r.DryRun {
			r.info("playlist not found — it would be created")
			return "", nil, nil
		}
		detail, err := r.API.CreatePlaylist(ctx, name)
		if err != nil {
			return "", nil, fmt.Errorf("create playlist %q: %w", name, err)
		}
		result.PlaylistCreated = true
		r.success("Created playlist %q", name)
		return detail.ID, nil, nil
	}

	detail, err := r.API.GetPlaylist(ctx, id)
	if err != nil {
		return "", nil, fmt.Errorf("load playlist %q: %w", name, err)
	}
	r.debug("found playlist %s (%s)", detail.ID, countLabel(len(detail.Tracks), "track"))
	return detail.ID, detail.Tracks, nil
}

func (r *Runner) printPlan(plan Plan) {
	r.step("Planning import")
	r.info("%d rows · %d matched in library · %d missing",
		plan.TotalRows, plan.Matched, plan.Unmatched)
	r.info("to add %d · to refresh %d · unchanged %d",
		len(plan.Adds), len(plan.Refreshes), plan.Unchanged)

	if r.Out == nil || !r.Out.Verbose() {
		return
	}
	for _, add := range plan.Adds {
		state := "unlinked"
		if add.Linked {
			state = "linked"
		}
		r.Out.Debug("+ %s — %s [%s]", add.Row.TrackName, add.Row.ArtistName, state)
	}
	for _, refresh := range plan.Refreshes {
		r.Out.Debug("~ %s — %s [relink]", refresh.Row.TrackName, refresh.Row.ArtistName)
	}
}

func (r *Runner) applyAdds(ctx context.Context, playlistID string, plan Plan, result *Result) error {
	if len(plan.Adds) == 0 {
		return nil
	}

	inputs := make([]api.TrackInput, len(plan.Adds))
	linked := linkedCount(plan.Adds)
	for i, add := range plan.Adds {
		inputs[i] = add.Track
	}
	result.AddedLinked = linked
	result.AddedUnlinked = len(inputs) - linked

	r.step("Adding %s (%d linked, %d unlinked)",
		countLabel(len(inputs), "track"), linked, result.AddedUnlinked)

	batchSize := r.batchSize()
	for start := 0; start < len(inputs); start += batchSize {
		end := start + batchSize
		if end > len(inputs) {
			end = len(inputs)
		}
		if _, err := r.API.AddTracks(ctx, playlistID, inputs[start:end], nil); err != nil {
			return fmt.Errorf("add tracks %d-%d: %w", start+1, end, err)
		}
		r.debug("added tracks %d-%d", start+1, end)
	}
	r.success("Added %s", countLabel(len(inputs), "track"))
	return nil
}

func (r *Runner) applyRefreshes(ctx context.Context, playlistID string, plan Plan, result *Result) error {
	if len(plan.Refreshes) == 0 {
		return nil
	}

	refreshes := append([]Refresh(nil), plan.Refreshes...)
	sort.SliceStable(refreshes, func(i, j int) bool {
		return refreshes[i].Existing.Position < refreshes[j].Existing.Position
	})

	r.step("Refreshing %s now present in the library", countLabel(len(refreshes), "track"))
	for _, refresh := range refreshes {
		// An unlinked entry cannot be patched into a linked one (the API
		// resolves a source from stored album/track metadata), so the entry is
		// replaced in place. The linked replacement is inserted first and the
		// old entry removed afterwards: if the insert fails the original entry
		// is still there, and if the remove fails we are left with a harmless
		// duplicate rather than a hole.
		position := refresh.Existing.Position
		if _, err := r.API.AddTracks(ctx, playlistID, []api.TrackInput{refresh.Track}, &position); err != nil {
			return fmt.Errorf("refresh %q: add linked entry: %w", refresh.Row.TrackName, err)
		}
		if err := r.API.RemoveTracks(ctx, playlistID, []string{refresh.Existing.ID}); err != nil {
			return fmt.Errorf("refresh %q: remove old entry (a duplicate may remain): %w", refresh.Row.TrackName, err)
		}
		result.Refreshed++
		r.debug("relinked %q — %s", refresh.Row.TrackName, refresh.Row.ArtistName)
	}
	r.success("Relinked %s", countLabel(result.Refreshed, "track"))
	return nil
}

func (r *Runner) resolveSources(ctx context.Context, playlistID string) {
	r.step("Resolving track sources")
	resolved, err := r.API.ResolveSources(ctx, playlistID)
	if err != nil {
		r.warn("source resolution failed: %v", err)
		return
	}
	r.debug("resolved sources for %d entries", len(resolved))
}

func (r *Runner) countPlayable(ctx context.Context, playlistID string, result *Result) {
	detail, err := r.API.GetPlaylist(ctx, playlistID)
	if err != nil {
		r.warn("could not reload the playlist for the summary: %v", err)
		return
	}
	for _, track := range detail.Tracks {
		if track.Linked() {
			result.Playable++
		} else {
			result.StillUnlinked++
		}
	}
}

func (r *Runner) batchSize() int {
	if r.BatchSize > 0 {
		return r.BatchSize
	}
	return defaultBatchSize
}

func (r *Runner) step(format string, args ...any) {
	if r.Out != nil {
		r.Out.Step(format, args...)
	}
}

func (r *Runner) success(format string, args ...any) {
	if r.Out != nil {
		r.Out.Success(format, args...)
	}
}

func (r *Runner) info(format string, args ...any) {
	if r.Out != nil {
		r.Out.Info(format, args...)
	}
}

func (r *Runner) warn(format string, args ...any) {
	if r.Out != nil {
		r.Out.Warn(format, args...)
	}
}

func (r *Runner) debug(format string, args ...any) {
	if r.Out != nil {
		r.Out.Debug(format, args...)
	}
}

func toMatcherTracks(tracks []api.LibraryTrack) []matcher.Track {
	result := make([]matcher.Track, len(tracks))
	for i, track := range tracks {
		result[i] = matcher.Track{
			ID:              track.ID,
			Title:           track.Title,
			AlbumID:         track.AlbumID,
			AlbumTitle:      track.AlbumTitle,
			ArtistID:        track.ArtistID,
			ArtistName:      track.ArtistName,
			DiscNumber:      track.DiscNumber,
			TrackNumber:     track.TrackNumber,
			DurationSeconds: track.DurationSeconds,
			Format:          track.Format,
		}
	}
	return result
}

func findPlaylistID(summaries []api.PlaylistSummary, name string) string {
	target := strings.TrimSpace(name)
	for _, summary := range summaries {
		candidate := strings.TrimSpace(summary.Name)
		if candidate == "" {
			continue // redacted playlist: no name visible
		}
		if strings.EqualFold(candidate, target) {
			return summary.ID
		}
	}
	return ""
}

func countLabel(n int, singular string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", singular)
	}
	return fmt.Sprintf("%d %ss", n, singular)
}

func linkedCount(adds []Add) int {
	linked := 0
	for _, add := range adds {
		if add.Linked {
			linked++
		}
	}
	return linked
}
