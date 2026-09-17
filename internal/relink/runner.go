package relink

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mimmo/needlectl/internal/api"
	"github.com/mimmo/needlectl/internal/link"
	"github.com/mimmo/needlectl/internal/matcher"
	"github.com/mimmo/needlectl/internal/output"
)

// maxListedRelinks caps how many entries the plan prints when verbose output is
// off, so relinking a large playlist does not flood the terminal.
const maxListedRelinks = 20

// API is the subset of the DroppedNeedle client the relinker needs.
type API interface {
	ListLibraryTracks(ctx context.Context) ([]api.LibraryTrack, error)
	ListPlaylists(ctx context.Context) ([]api.PlaylistSummary, error)
	GetPlaylist(ctx context.Context, id string) (*api.PlaylistDetail, error)
	AddTracks(ctx context.Context, playlistID string, tracks []api.TrackInput, position *int) ([]api.PlaylistTrack, error)
	RemoveTracks(ctx context.Context, playlistID string, trackIDs []string) error
	ResolveSources(ctx context.Context, playlistID string) (map[string][]string, error)
}

// Target identifies the playlist to relink, either by id or by name.
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

// Runner relinks the unplayable entries of one playlist.
type Runner struct {
	API     API
	Out     *output.Printer
	Resolve bool
	DryRun  bool
}

// Result summarizes what a relink run did (or would do).
type Result struct {
	PlaylistID    string
	PlaylistName  string
	LibrarySize   int
	Total         int
	Playable      int
	Relinked      int
	Unmatched     int
	PlayableAfter int
	StillUnlinked int
}

// Run relinks the unplayable entries of target.
func (r *Runner) Run(ctx context.Context, target Target) (Result, error) {
	result := Result{PlaylistName: target.Name}

	index, err := r.loadLibrary(ctx, &result)
	if err != nil {
		return result, err
	}

	detail, err := r.loadPlaylist(ctx, target)
	if err != nil {
		return result, err
	}
	result.PlaylistID = detail.ID
	result.PlaylistName = detail.Name

	plan := BuildPlan(detail.Tracks, index)
	result.Total = plan.Total
	result.Playable = plan.Playable
	result.Unmatched = plan.Unmatched

	r.printPlan(plan)
	if r.DryRun {
		result.Relinked = len(plan.Relinks)
		return result, nil
	}

	if err := r.apply(ctx, detail.ID, plan, &result); err != nil {
		return result, err
	}
	if r.Resolve {
		r.resolveSources(ctx, detail.ID)
	}
	r.countPlayable(ctx, detail.ID, &result)
	return result, nil
}

func (r *Runner) loadLibrary(ctx context.Context, result *Result) (*matcher.Index, error) {
	r.step("Loading DroppedNeedle library")
	tracks, err := r.API.ListLibraryTracks(ctx)
	if err != nil {
		return nil, fmt.Errorf("load library: %w", err)
	}
	index := link.Index(tracks)
	result.LibrarySize = index.Len()
	r.info("%s indexed", output.CountLabel(index.Len(), "track"))
	return index, nil
}

func (r *Runner) loadPlaylist(ctx context.Context, target Target) (*api.PlaylistDetail, error) {
	id, err := r.playlistID(ctx, target)
	if err != nil {
		return nil, err
	}

	detail, err := r.API.GetPlaylist(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load playlist %q: %w", id, err)
	}
	r.debug("found playlist %s (%s)", detail.ID, output.CountLabel(len(detail.Tracks), "track"))
	return detail, nil
}

// playlistID resolves the target to a playlist id, refusing to guess when a
// name matches several playlists.
func (r *Runner) playlistID(ctx context.Context, target Target) (string, error) {
	if id := strings.TrimSpace(target.ID); id != "" {
		r.step("Using playlist id %s", id)
		return id, nil
	}

	name := strings.TrimSpace(target.Name)
	if name == "" {
		return "", errors.New("no playlist given")
	}

	r.step("Resolving playlist %q", name)
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

func (r *Runner) printPlan(plan Plan) {
	r.step("Planning relink")
	r.info("%d entries · %d already playable · %d to relink · %d unmatched",
		plan.Total, plan.Playable, len(plan.Relinks), plan.Unmatched)

	if r.Out == nil {
		return
	}

	verbose := r.Out.Verbose()
	for i, item := range plan.Relinks {
		if !verbose && i == maxListedRelinks {
			r.info("… and %d more (use --verbose to list them all)", len(plan.Relinks)-i)
			break
		}
		line := entryLabel(item.Existing)
		if verbose {
			// The entry is matched by title/artist/album but linked by file id,
			// so show which library file the match resolved to.
			line = fmt.Sprintf("%s → %s", line, libraryLabel(item.Library))
		}
		r.info("~ %s", line)
	}
}

// entryLabel renders a playlist entry as "title — artist".
func entryLabel(track api.PlaylistTrack) string {
	title := strings.TrimSpace(track.TrackName)
	if title == "" {
		title = "(untitled)"
	}
	if strings.TrimSpace(track.ArtistName) == "" {
		return title
	}
	return fmt.Sprintf("%s — %s", title, track.ArtistName)
}

// libraryLabel describes the matched library file.
func libraryLabel(track matcher.Track) string {
	parts := []string{track.ID}
	if album := strings.TrimSpace(track.AlbumTitle); album != "" {
		parts = append(parts, album)
	}
	if track.Format != "" {
		parts = append(parts, track.Format)
	}
	return strings.Join(parts, " · ")
}

// apply replaces every planned entry in ascending position order, which keeps
// the relative order of the untouched entries intact.
func (r *Runner) apply(ctx context.Context, playlistID string, plan Plan, result *Result) error {
	if len(plan.Relinks) == 0 {
		return nil
	}

	relinks := append([]Relink(nil), plan.Relinks...)
	sort.SliceStable(relinks, func(i, j int) bool {
		return relinks[i].Existing.Position < relinks[j].Existing.Position
	})

	r.step("Linking %s to the library", output.CountLabel(len(relinks), "track"))
	for _, item := range relinks {
		if err := link.Replace(ctx, r.API, playlistID, item.Existing, item.Track); err != nil {
			return fmt.Errorf("relink %q: %w", item.Existing.TrackName, err)
		}
		result.Relinked++
		r.debug("relinked %q — %s", item.Existing.TrackName, item.Existing.ArtistName)
	}
	r.success("Relinked %s", output.CountLabel(result.Relinked, "track"))
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
			result.PlayableAfter++
		} else {
			result.StillUnlinked++
		}
	}
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
