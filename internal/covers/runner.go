package covers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mimnix/needlectl/internal/api"
	"github.com/mimnix/needlectl/internal/link"
	"github.com/mimnix/needlectl/internal/output"
)

// maxListedRepairs caps how many entries the plan prints unless verbose output
// is on, so repairing a large playlist does not flood the terminal.
const maxListedRepairs = 20

// API is the subset of the DroppedNeedle client the cover repair needs.
type API interface {
	ListLibraryTracks(ctx context.Context) ([]api.LibraryTrack, error)
	ListPlaylists(ctx context.Context) ([]api.PlaylistSummary, error)
	GetPlaylist(ctx context.Context, id string) (*api.PlaylistDetail, error)
	AddTracks(ctx context.Context, playlistID string, tracks []api.TrackInput, position *int) ([]api.PlaylistTrack, error)
	RemoveTracks(ctx context.Context, playlistID string, trackIDs []string) error
}

// Target identifies the playlist to repair, either by id or by name.
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

// Result summarizes the repairs of one playlist.
type Result struct {
	PlaylistID     string
	PlaylistName   string
	LibrarySize    int
	Total          int
	Repaired       int
	Unchanged      int
	NoLibrary      int
	NoReleaseGroup int
}

// Runner repairs the cover URLs of playlist entries.
type Runner struct {
	API    API
	Out    *output.Printer
	DryRun bool
	// All repairs every playlist the caller owns instead of one target.
	All bool
	// Pause is slept between replacements so a bulk repair stays under the
	// server's request limit. Zero disables it (tests).
	Pause time.Duration
}

// Run repairs the target playlist, or every owned playlist when All is set.
func (r *Runner) Run(ctx context.Context, target Target) ([]Result, error) {
	tracks, err := r.API.ListLibraryTracks(ctx)
	if err != nil {
		return nil, fmt.Errorf("load library: %w", err)
	}
	library := Index(tracks)
	r.step("Loading DroppedNeedle library")
	r.info("%s indexed", output.CountLabel(len(library), "track"))

	if r.All {
		return r.runAll(ctx, library)
	}

	id, err := r.playlistID(ctx, target)
	if err != nil {
		return nil, err
	}
	detail, err := r.API.GetPlaylist(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load playlist %s: %w", id, err)
	}
	result, err := r.repairPlaylist(ctx, detail, library)
	if err != nil {
		return nil, err
	}
	return []Result{result}, nil
}

func (r *Runner) runAll(ctx context.Context, library map[string]api.LibraryTrack) ([]Result, error) {
	summaries, err := r.API.ListPlaylists(ctx)
	if err != nil {
		return nil, fmt.Errorf("list playlists: %w", err)
	}

	var results []Result
	for _, summary := range summaries {
		if !summary.IsOwner || summary.IsRedacted || strings.TrimSpace(summary.Name) == "" {
			continue
		}
		detail, err := r.API.GetPlaylist(ctx, summary.ID)
		if err != nil {
			return results, fmt.Errorf("load playlist %s: %w", summary.ID, err)
		}
		result, err := r.repairPlaylist(ctx, detail, library)
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (r *Runner) repairPlaylist(ctx context.Context, detail *api.PlaylistDetail, library map[string]api.LibraryTrack) (Result, error) {
	result := Result{
		PlaylistID:   detail.ID,
		PlaylistName: detail.Name,
		LibrarySize:  len(library),
		Total:        len(detail.Tracks),
	}

	plan := BuildPlan(detail.Tracks, library)
	result.Unchanged = plan.Unchanged
	result.NoLibrary = plan.NoLibrary
	result.NoReleaseGroup = plan.NoReleaseGroup

	r.printPlan(detail.Name, plan)
	if r.DryRun {
		result.Repaired = len(plan.Repairs)
		return result, nil
	}

	for i, repair := range plan.Repairs {
		if i > 0 && r.Pause > 0 {
			select {
			case <-ctx.Done():
				return result, ctx.Err()
			case <-time.After(r.Pause):
			}
		}
		if err := link.Replace(ctx, r.API, detail.ID, repair.Existing, repair.Input); err != nil {
			return result, fmt.Errorf("repair cover of %q: %w", repair.Existing.TrackName, err)
		}
		result.Repaired++
		if (i+1)%10 == 0 {
			r.info("repaired %d of %d", i+1, len(plan.Repairs))
		}
	}
	return result, nil
}

func (r *Runner) printPlan(name string, plan Plan) {
	r.step("Checking covers of %q", name)
	r.info("%d entries · %d to repair · %d already right · %d not in the library · %d without a release group",
		plan.Total, len(plan.Repairs), plan.Unchanged, plan.NoLibrary, plan.NoReleaseGroup)

	for i, repair := range plan.Repairs {
		if i >= maxListedRepairs && !r.verbose() {
			r.info("… and %d more (use --verbose to list them all)", len(plan.Repairs)-i)
			break
		}
		r.info("~ %s — %s", repair.Existing.TrackName, repair.Existing.ArtistName)
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
