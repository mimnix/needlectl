package relink

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/mimmo/needlectl/internal/api"
	"github.com/mimmo/needlectl/internal/matcher"
	"github.com/mimmo/needlectl/internal/output"
)

// fakeAPI is an in-memory DroppedNeedle used to exercise Runner end to end.
type fakeAPI struct {
	library      []api.LibraryTrack
	playlists    []api.PlaylistSummary
	details      map[string]*api.PlaylistDetail
	sequence     int
	entrySeq     int
	resolveCalls int
	addedBatches [][]api.TrackInput
	removedIDs   []string
}

func newFakeAPI(library ...api.LibraryTrack) *fakeAPI {
	return &fakeAPI{library: library, details: map[string]*api.PlaylistDetail{}}
}

func (f *fakeAPI) seedPlaylist(name string, tracks ...api.PlaylistTrack) string {
	f.sequence++
	id := fmt.Sprintf("pl-%d", f.sequence)
	detail := &api.PlaylistDetail{ID: id, Name: name, Tracks: tracks}
	f.reindex(detail)
	f.details[id] = detail
	f.playlists = append(f.playlists, api.PlaylistSummary{ID: id, Name: name, TrackCount: len(tracks)})
	return id
}

func (f *fakeAPI) ListLibraryTracks(context.Context) ([]api.LibraryTrack, error) {
	return f.library, nil
}

func (f *fakeAPI) ListPlaylists(context.Context) ([]api.PlaylistSummary, error) {
	return f.playlists, nil
}

func (f *fakeAPI) GetPlaylist(_ context.Context, id string) (*api.PlaylistDetail, error) {
	detail, ok := f.details[id]
	if !ok {
		return nil, fmt.Errorf("playlist %s not found", id)
	}
	return detail, nil
}

func (f *fakeAPI) AddTracks(_ context.Context, playlistID string, tracks []api.TrackInput, position *int) ([]api.PlaylistTrack, error) {
	detail := f.details[playlistID]
	f.addedBatches = append(f.addedBatches, tracks)

	for _, input := range tracks {
		f.entrySeq++
		entry := api.PlaylistTrack{
			ID:               fmt.Sprintf("entry-%d", f.entrySeq),
			TrackName:        input.TrackName,
			ArtistName:       input.ArtistName,
			AlbumName:        input.AlbumName,
			TrackSourceID:    input.TrackSourceID,
			LibraryFileID:    input.TrackSourceID,
			SourceType:       input.SourceType,
			AvailableSources: input.AvailableSources,
		}
		if position != nil && *position >= 0 && *position <= len(detail.Tracks) {
			at := *position
			detail.Tracks = append(detail.Tracks, api.PlaylistTrack{})
			copy(detail.Tracks[at+1:], detail.Tracks[at:])
			detail.Tracks[at] = entry
		} else {
			detail.Tracks = append(detail.Tracks, entry)
		}
	}
	f.reindex(detail)
	return nil, nil
}

func (f *fakeAPI) RemoveTracks(_ context.Context, playlistID string, trackIDs []string) error {
	detail := f.details[playlistID]
	remove := make(map[string]bool, len(trackIDs))
	for _, id := range trackIDs {
		remove[id] = true
	}

	var kept []api.PlaylistTrack
	for _, entry := range detail.Tracks {
		if remove[entry.ID] {
			f.removedIDs = append(f.removedIDs, entry.ID)
			continue
		}
		kept = append(kept, entry)
	}
	detail.Tracks = kept
	f.reindex(detail)
	return nil
}

func (f *fakeAPI) ResolveSources(context.Context, string) (map[string][]string, error) {
	f.resolveCalls++
	return map[string][]string{}, nil
}

func (f *fakeAPI) reindex(detail *api.PlaylistDetail) {
	for i := range detail.Tracks {
		detail.Tracks[i].Position = i
	}
	detail.TrackCount = len(detail.Tracks)
}

func libraryTrack(id, title, artist, album string) api.LibraryTrack {
	return api.LibraryTrack{
		ID: id, Title: title, ArtistName: artist, AlbumTitle: album,
		AlbumID: "rg-" + id, ArtistID: "ar-" + id,
	}
}

func TestRunnerRelinksUnplayableEntriesAndKeepsOrder(t *testing.T) {
	apiFake := newFakeAPI(
		libraryTrack("file-1", "Disorder", "神崎エルザ", "Elza"),
		libraryTrack("file-2", "Rising Hope", "LiSA", "Launcher"),
	)
	apiFake.seedPlaylist("Anime",
		playableEntry("old-0", "Intro", "LiSA", "Launcher", 0),
		unplayableEntry("old-1", "Disorder", "神崎エルザ", "Elza", 1),
		unplayableEntry("old-2", "Rising Hope", "LiSA", "Launcher", 2),
	)
	runner := &Runner{API: apiFake, Resolve: true}

	result, err := runner.Run(context.Background(), Target{Name: "Anime"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if result.Total != 3 || result.Playable != 1 || result.Relinked != 2 || result.Unmatched != 0 {
		t.Errorf("total/playable/relinked/unmatched = %d/%d/%d/%d, want 3/1/2/0",
			result.Total, result.Playable, result.Relinked, result.Unmatched)
	}
	if result.PlayableAfter != 3 || result.StillUnlinked != 0 {
		t.Errorf("playable/unlinked after = %d/%d, want 3/0", result.PlayableAfter, result.StillUnlinked)
	}
	if len(apiFake.removedIDs) != 2 || apiFake.removedIDs[0] != "old-1" || apiFake.removedIDs[1] != "old-2" {
		t.Errorf("removedIDs = %v, want [old-1 old-2]", apiFake.removedIDs)
	}
	if apiFake.resolveCalls != 1 {
		t.Errorf("resolveCalls = %d, want 1", apiFake.resolveCalls)
	}

	detail, _ := apiFake.GetPlaylist(context.Background(), result.PlaylistID)
	want := []string{"Intro", "Disorder", "Rising Hope"}
	if len(detail.Tracks) != len(want) {
		t.Fatalf("playlist has %d entries, want %d", len(detail.Tracks), len(want))
	}
	for i, title := range want {
		if detail.Tracks[i].TrackName != title {
			t.Errorf("entry %d = %q, want %q", i, detail.Tracks[i].TrackName, title)
		}
	}
}

func TestRunnerLeavesPlayableEntriesAlone(t *testing.T) {
	apiFake := newFakeAPI(libraryTrack("file-1", "Disorder", "神崎エルザ", "Elza"))
	apiFake.seedPlaylist("Anime", playableEntry("old-0", "Disorder", "神崎エルザ", "Elza", 0))
	runner := &Runner{API: apiFake, Resolve: true}

	result, err := runner.Run(context.Background(), Target{Name: "Anime"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if result.Relinked != 0 || len(apiFake.addedBatches) != 0 || len(apiFake.removedIDs) != 0 {
		t.Errorf("playable playlist was modified: %+v", result)
	}
	if result.PlayableAfter != 1 {
		t.Errorf("PlayableAfter = %d, want 1", result.PlayableAfter)
	}
}

func TestRunnerDryRunDoesNotModify(t *testing.T) {
	apiFake := newFakeAPI(libraryTrack("file-1", "Disorder", "神崎エルザ", "Elza"))
	apiFake.seedPlaylist("Anime", unplayableEntry("old-1", "Disorder", "神崎エルザ", "Elza", 0))
	runner := &Runner{API: apiFake, DryRun: true, Resolve: true}

	result, err := runner.Run(context.Background(), Target{Name: "Anime"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if result.Relinked != 1 {
		t.Errorf("Relinked = %d, want the 1 planned relink", result.Relinked)
	}
	if len(apiFake.addedBatches) != 0 || len(apiFake.removedIDs) != 0 {
		t.Error("dry run modified the playlist")
	}
	if apiFake.resolveCalls != 0 {
		t.Errorf("resolveCalls = %d, want 0", apiFake.resolveCalls)
	}

	detail, _ := apiFake.GetPlaylist(context.Background(), "pl-1")
	if detail.Tracks[0].Linked() {
		t.Error("dry run linked an entry")
	}
}

func TestRunnerSkipsResolutionWhenDisabled(t *testing.T) {
	apiFake := newFakeAPI(libraryTrack("file-1", "Disorder", "神崎エルザ", "Elza"))
	apiFake.seedPlaylist("Anime", unplayableEntry("old-1", "Disorder", "神崎エルザ", "Elza", 0))
	runner := &Runner{API: apiFake, Resolve: false}

	if _, err := runner.Run(context.Background(), Target{Name: "Anime"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if apiFake.resolveCalls != 0 {
		t.Errorf("resolveCalls = %d, want 0", apiFake.resolveCalls)
	}
}

func TestRunnerResolvesPlaylistByID(t *testing.T) {
	apiFake := newFakeAPI(libraryTrack("file-1", "Disorder", "神崎エルザ", "Elza"))
	apiFake.seedPlaylist("Anime", unplayableEntry("old-1", "Disorder", "神崎エルザ", "Elza", 0))
	runner := &Runner{API: apiFake}

	result, err := runner.Run(context.Background(), Target{ID: "pl-1"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.PlaylistID != "pl-1" {
		t.Errorf("PlaylistID = %q, want pl-1", result.PlaylistID)
	}
	if result.Relinked != 1 {
		t.Errorf("Relinked = %d, want 1", result.Relinked)
	}
}

func TestRunnerReportsUnknownPlaylist(t *testing.T) {
	apiFake := newFakeAPI()

	_, err := (&Runner{API: apiFake}).Run(context.Background(), Target{Name: "Nope"})

	if err == nil {
		t.Fatal("Run returned nil error, want a failure")
	}
	var ambiguous *AmbiguousPlaylistError
	if errors.As(err, &ambiguous) {
		t.Errorf("error = %v, want a plain not-found error", err)
	}
}

func TestRunnerRefusesAmbiguousPlaylistName(t *testing.T) {
	apiFake := newFakeAPI()
	apiFake.seedPlaylist("Anime", unplayableEntry("old-1", "Disorder", "神崎エルザ", "Elza", 0))
	apiFake.seedPlaylist("Anime")

	_, err := (&Runner{API: apiFake}).Run(context.Background(), Target{Name: "Anime"})

	var ambiguous *AmbiguousPlaylistError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("error = %v, want *AmbiguousPlaylistError", err)
	}
	if len(ambiguous.Matches) != 2 {
		t.Errorf("matches = %d, want 2", len(ambiguous.Matches))
	}
	if len(apiFake.addedBatches) != 0 || len(apiFake.removedIDs) != 0 {
		t.Error("an ambiguous name must not modify any playlist")
	}
}

func TestRunnerRequiresATarget(t *testing.T) {
	if _, err := (&Runner{API: newFakeAPI()}).Run(context.Background(), Target{}); err == nil {
		t.Fatal("Run returned nil error, want a failure")
	}
}

func TestPrintPlanListsTheTracksToRelink(t *testing.T) {
	var buffer bytes.Buffer
	runner := &Runner{Out: output.New(&buffer, &buffer, output.Options{})}

	runner.printPlan(Plan{
		Total: 3, Playable: 1, Unmatched: 1,
		Relinks: []Relink{{
			Existing: api.PlaylistTrack{TrackName: "Disorder", ArtistName: "神崎エルザ", Position: 1},
			Library:  matcher.Track{ID: "file-1", AlbumTitle: "Elza", Format: "FLAC"},
		}},
	})

	got := buffer.String()
	if !strings.Contains(got, "~ Disorder — 神崎エルザ") {
		t.Errorf("plan output %q does not list the track to relink", got)
	}
	if strings.Contains(got, "file-1") {
		t.Errorf("non-verbose output %q should not include library internals", got)
	}
}

func TestPrintPlanShowsTheMatchedLibraryFileWhenVerbose(t *testing.T) {
	var buffer bytes.Buffer
	runner := &Runner{Out: output.New(&buffer, &buffer, output.Options{Verbose: true})}

	runner.printPlan(Plan{Total: 1, Relinks: []Relink{{
		Existing: api.PlaylistTrack{TrackName: "Disorder", ArtistName: "神崎エルザ"},
		Library:  matcher.Track{ID: "file-1", AlbumTitle: "Elza", Format: "FLAC"},
	}}})

	got := buffer.String()
	if !strings.Contains(got, "~ Disorder — 神崎エルザ → file-1 · Elza · FLAC") {
		t.Errorf("verbose plan output %q does not show the matched library file", got)
	}
}

func TestPrintPlanTruncatesLongListsUnlessVerbose(t *testing.T) {
	relinks := make([]Relink, 0, maxListedRelinks+5)
	for i := 0; i < maxListedRelinks+5; i++ {
		relinks = append(relinks, Relink{Existing: api.PlaylistTrack{ID: fmt.Sprintf("e-%d", i)}})
	}

	var quiet bytes.Buffer
	(&Runner{Out: output.New(&quiet, &quiet, output.Options{})}).printPlan(Plan{Total: len(relinks), Relinks: relinks})
	if !strings.Contains(quiet.String(), "… and 5 more") {
		t.Errorf("quiet plan output %q does not report the truncation", quiet.String())
	}

	var verbose bytes.Buffer
	(&Runner{Out: output.New(&verbose, &verbose, output.Options{Verbose: true})}).printPlan(Plan{Total: len(relinks), Relinks: relinks})
	if strings.Contains(verbose.String(), "more") {
		t.Errorf("verbose plan output %q should list every entry", verbose.String())
	}
	if count := strings.Count(verbose.String(), "~ "); count != len(relinks) {
		t.Errorf("verbose plan listed %d entries, want %d", count, len(relinks))
	}
}

func TestRunnerIsIdempotent(t *testing.T) {
	apiFake := newFakeAPI(libraryTrack("file-1", "Disorder", "神崎エルザ", "Elza"))
	apiFake.seedPlaylist("Anime", unplayableEntry("old-1", "Disorder", "神崎エルザ", "Elza", 0))
	runner := &Runner{API: apiFake}

	if _, err := runner.Run(context.Background(), Target{Name: "Anime"}); err != nil {
		t.Fatalf("first Run returned error: %v", err)
	}

	result, err := runner.Run(context.Background(), Target{Name: "Anime"})
	if err != nil {
		t.Fatalf("second Run returned error: %v", err)
	}
	if result.Relinked != 0 {
		t.Errorf("second run relinked %d tracks, want 0", result.Relinked)
	}
	if result.Playable != 1 || result.PlayableAfter != 1 {
		t.Errorf("playable/after = %d/%d, want 1/1", result.Playable, result.PlayableAfter)
	}
}
