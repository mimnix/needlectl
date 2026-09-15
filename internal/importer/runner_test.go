package importer

import (
	"context"
	"fmt"
	"testing"

	"github.com/mimmo/needlectl/internal/api"
	"github.com/mimmo/needlectl/internal/playlist"
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

func (f *fakeAPI) CreatePlaylist(_ context.Context, name string) (*api.PlaylistDetail, error) {
	id := f.seedPlaylist(name)
	return f.details[id], nil
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
			AlbumID:          input.AlbumID,
			ArtistID:         input.ArtistID,
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

func libraryTrackFor(row playlist.Row, id string) api.LibraryTrack {
	return api.LibraryTrack{
		ID:         id,
		Title:      row.TrackName,
		AlbumTitle: row.AlbumName,
		ArtistName: row.ArtistName,
		AlbumID:    "rg-" + id,
		ArtistID:   "ar-" + id,
	}
}

func TestRunnerImportsAndLinksMatchingTracks(t *testing.T) {
	matched := testRow("Disorder", "神崎エルザ", "Elza")
	missing := testRow("Missing Song", "Nobody", "Nowhere")
	apiFake := newFakeAPI(libraryTrackFor(matched, "file-1"))
	runner := &Runner{API: apiFake, ResolveSources: true}

	result, err := runner.Run(context.Background(), "Anime", []playlist.Row{matched, missing})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if !result.PlaylistCreated {
		t.Error("PlaylistCreated = false, want true")
	}
	if result.PlaylistID == "" {
		t.Error("PlaylistID is empty")
	}
	if result.AddedLinked != 1 || result.AddedUnlinked != 1 {
		t.Errorf("added linked/unlinked = %d/%d, want 1/1", result.AddedLinked, result.AddedUnlinked)
	}
	if result.Playable != 1 || result.StillUnlinked != 1 {
		t.Errorf("playable/unlinked = %d/%d, want 1/1", result.Playable, result.StillUnlinked)
	}
	if apiFake.resolveCalls != 1 {
		t.Errorf("resolveCalls = %d, want 1", apiFake.resolveCalls)
	}
}

func TestRunnerIsIdempotent(t *testing.T) {
	matched := testRow("Disorder", "神崎エルザ", "Elza")
	missing := testRow("Missing Song", "Nobody", "Nowhere")
	apiFake := newFakeAPI(libraryTrackFor(matched, "file-1"))
	runner := &Runner{API: apiFake, ResolveSources: true}

	if _, err := runner.Run(context.Background(), "Anime", []playlist.Row{matched, missing}); err != nil {
		t.Fatalf("first Run returned error: %v", err)
	}

	result, err := runner.Run(context.Background(), "Anime", []playlist.Row{matched, missing})
	if err != nil {
		t.Fatalf("second Run returned error: %v", err)
	}

	if result.PlaylistCreated {
		t.Error("PlaylistCreated = true on the second run, want false")
	}
	added := result.AddedLinked + result.AddedUnlinked
	if added != 0 {
		t.Errorf("second run added %d tracks, want 0", added)
	}
	if result.Refreshed != 0 {
		t.Errorf("second run refreshed %d tracks, want 0", result.Refreshed)
	}
	if result.Unchanged != 2 {
		t.Errorf("Unchanged = %d, want 2", result.Unchanged)
	}
}

func TestRunnerRefreshesPreviouslyUnlinkedEntry(t *testing.T) {
	row := testRow("Disorder", "神崎エルザ", "Elza")
	apiFake := newFakeAPI(libraryTrackFor(row, "file-1"))
	apiFake.seedPlaylist("Anime", api.PlaylistTrack{
		ID: "entry-old", Position: 0, TrackName: row.TrackName, ArtistName: row.ArtistName, AlbumName: row.AlbumName,
	})
	runner := &Runner{API: apiFake}

	result, err := runner.Run(context.Background(), "Anime", []playlist.Row{row})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if result.Refreshed != 1 {
		t.Fatalf("Refreshed = %d, want 1", result.Refreshed)
	}
	if len(apiFake.removedIDs) != 1 || apiFake.removedIDs[0] != "entry-old" {
		t.Errorf("removedIDs = %v, want [entry-old]", apiFake.removedIDs)
	}
	if result.StillUnlinked != 0 {
		t.Errorf("StillUnlinked = %d, want 0", result.StillUnlinked)
	}
}

func TestRunnerRefreshesMultipleEntriesInOrder(t *testing.T) {
	first := testRow("Disorder", "神崎エルザ", "Elza")
	second := testRow("Rising Hope", "LiSA", "Launcher")
	apiFake := newFakeAPI(libraryTrackFor(first, "file-1"), libraryTrackFor(second, "file-2"))
	apiFake.seedPlaylist("Anime",
		api.PlaylistTrack{ID: "old-1", Position: 0, TrackName: first.TrackName, ArtistName: first.ArtistName, AlbumName: first.AlbumName},
		api.PlaylistTrack{ID: "old-2", Position: 1, TrackName: second.TrackName, ArtistName: second.ArtistName, AlbumName: second.AlbumName},
	)
	runner := &Runner{API: apiFake}

	result, err := runner.Run(context.Background(), "Anime", []playlist.Row{first, second})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Refreshed != 2 {
		t.Fatalf("Refreshed = %d, want 2", result.Refreshed)
	}
	if len(apiFake.removedIDs) != 2 {
		t.Fatalf("removedIDs = %v, want both old entries", apiFake.removedIDs)
	}

	detail, _ := apiFake.GetPlaylist(context.Background(), result.PlaylistID)
	if len(detail.Tracks) != 2 {
		t.Fatalf("final playlist has %d entries, want 2", len(detail.Tracks))
	}
	if detail.Tracks[0].TrackName != first.TrackName || detail.Tracks[1].TrackName != second.TrackName {
		t.Errorf("order not preserved: %+v", detail.Tracks)
	}
	for _, entry := range detail.Tracks {
		if !entry.Linked() {
			t.Errorf("entry %q was not linked", entry.TrackName)
		}
	}
}

func TestRunnerDryRunDoesNotModify(t *testing.T) {
	row := testRow("Disorder", "神崎エルザ", "Elza")
	apiFake := newFakeAPI(libraryTrackFor(row, "file-1"))
	runner := &Runner{API: apiFake, DryRun: true, ResolveSources: true}

	result, err := runner.Run(context.Background(), "Anime", []playlist.Row{row})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if len(apiFake.playlists) != 0 {
		t.Errorf("dry run created %d playlists, want 0", len(apiFake.playlists))
	}
	if result.PlaylistID != "" {
		t.Errorf("PlaylistID = %q, want empty", result.PlaylistID)
	}
	if result.AddedLinked != 1 {
		t.Errorf("AddLinked = %d, want 1", result.AddedLinked)
	}
	if apiFake.resolveCalls != 0 {
		t.Errorf("resolveCalls = %d, want 0", apiFake.resolveCalls)
	}
}

func TestRunnerSkipsResolutionWhenDisabled(t *testing.T) {
	row := testRow("Disorder", "神崎エルザ", "Elza")
	apiFake := newFakeAPI(libraryTrackFor(row, "file-1"))
	runner := &Runner{API: apiFake, ResolveSources: false}

	if _, err := runner.Run(context.Background(), "Anime", []playlist.Row{row}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if apiFake.resolveCalls != 0 {
		t.Errorf("resolveCalls = %d, want 0", apiFake.resolveCalls)
	}
}
