package covers

import (
	"context"
	"errors"
	"testing"

	"github.com/mimnix/needlectl/internal/api"
)

func TestBuildPlanRepairsWrongAndMissingCovers(t *testing.T) {
	library := Index([]api.LibraryTrack{
		{ID: "t-1", AlbumID: "local-1", ReleaseGroupMBID: strPtr("rg-1")},
		{ID: "t-2", AlbumID: "local-2", ReleaseGroupMBID: strPtr("rg-2")},
		{ID: "t-3", AlbumID: "local-3"}, // no release group
	})

	entries := []api.PlaylistTrack{
		// Stored URL points at the local album id: broken.
		{ID: "e-1", Position: 0, TrackName: "Disorder", TrackSourceID: strPtr("t-1"), CoverURL: strPtr("/api/v3/covers/release-group/local-1?size=250")},
		// Already right.
		{ID: "e-2", Position: 1, TrackName: "Rising Hope", TrackSourceID: strPtr("t-2"), CoverURL: strPtr("/api/v3/covers/release-group/rg-2?size=250")},
		// No cover stored at all.
		{ID: "e-3", Position: 2, TrackName: "Empty MERMAiD", TrackSourceID: strPtr("t-1")},
		// Only library_file_id is set.
		{ID: "e-4", Position: 3, TrackName: "JUMP UP !!", LibraryFileID: strPtr("t-1")},
		// Not in the library.
		{ID: "e-5", Position: 4, TrackName: "Unlinked", TrackSourceID: strPtr("missing")},
		// Album without a release group.
		{ID: "e-6", Position: 5, TrackName: "No group", TrackSourceID: strPtr("t-3")},
	}

	plan := BuildPlan(entries, library)

	if plan.Total != 6 {
		t.Errorf("Total = %d, want 6", plan.Total)
	}
	if plan.Unchanged != 1 {
		t.Errorf("Unchanged = %d, want 1", plan.Unchanged)
	}
	if plan.NoLibrary != 1 {
		t.Errorf("NoLibrary = %d, want 1", plan.NoLibrary)
	}
	if plan.NoReleaseGroup != 1 {
		t.Errorf("NoReleaseGroup = %d, want 1", plan.NoReleaseGroup)
	}
	if len(plan.Repairs) != 3 {
		t.Fatalf("Repairs = %d, want 3", len(plan.Repairs))
	}

	want := map[string]string{
		"e-1": "/api/v3/covers/release-group/rg-1?size=250",
		"e-3": "/api/v3/covers/release-group/rg-1?size=250",
		"e-4": "/api/v3/covers/release-group/rg-1?size=250",
	}
	for _, repair := range plan.Repairs {
		if got, ok := want[repair.Existing.ID]; !ok || repair.New != got {
			t.Errorf("repair of %s = %q, want %q", repair.Existing.ID, repair.New, got)
		}
		if repair.Input.CoverURL == nil || *repair.Input.CoverURL != repair.New {
			t.Errorf("repair of %s did not carry the new URL", repair.Existing.ID)
		}
	}
}

func TestRepairInputPreservesTheEntry(t *testing.T) {
	duration := 215.6
	plan := BuildPlan([]api.PlaylistTrack{{
		ID:               "e-1",
		Position:         4,
		TrackName:        "Disorder",
		ArtistName:       "神崎エルザ",
		AlbumName:        "Elza",
		AlbumID:          strPtr("local-1"),
		ArtistID:         strPtr("ar-1"),
		TrackSourceID:    strPtr("t-1"),
		CoverURL:         strPtr("broken"),
		SourceType:       "local",
		AvailableSources: []string{"local"},
		Format:           strPtr("flac"),
		TrackNumber:      intPtr(4),
		DiscNumber:       intPtr(1),
		Duration:         &duration,
		RecordingMBID:    strPtr("rec-1"),
		PlexRatingKey:    strPtr("plex-1"),
	}}, Index([]api.LibraryTrack{{ID: "t-1", AlbumID: "local-1", ReleaseGroupMBID: strPtr("rg-1")}}))

	if len(plan.Repairs) != 1 {
		t.Fatalf("Repairs = %d, want 1", len(plan.Repairs))
	}
	input := plan.Repairs[0].Input

	if input.TrackName != "Disorder" || input.ArtistName != "神崎エルザ" || input.AlbumName != "Elza" {
		t.Errorf("metadata not preserved: %+v", input)
	}
	if input.AlbumID == nil || *input.AlbumID != "local-1" {
		t.Errorf("AlbumID = %v, want the stored local id", input.AlbumID)
	}
	if input.ArtistID == nil || *input.ArtistID != "ar-1" {
		t.Errorf("ArtistID = %v, want ar-1", input.ArtistID)
	}
	if input.TrackSourceID == nil || *input.TrackSourceID != "t-1" {
		t.Errorf("TrackSourceID = %v, want t-1", input.TrackSourceID)
	}
	if input.SourceType != "local" || len(input.AvailableSources) != 1 {
		t.Errorf("source fields not preserved: %+v", input)
	}
	if input.Format == nil || *input.Format != "flac" {
		t.Errorf("Format = %v, want flac", input.Format)
	}
	if input.TrackNumber == nil || *input.TrackNumber != 4 || input.DiscNumber == nil || *input.DiscNumber != 1 {
		t.Errorf("disc/track numbers not preserved: %v/%v", input.DiscNumber, input.TrackNumber)
	}
	if input.Duration == nil || *input.Duration != 216 {
		t.Errorf("Duration = %v, want 216 (rounded)", input.Duration)
	}
	if input.RecordingMBID == nil || *input.RecordingMBID != "rec-1" {
		t.Errorf("RecordingMBID = %v, want rec-1", input.RecordingMBID)
	}
	if input.PlexRatingKey == nil || *input.PlexRatingKey != "plex-1" {
		t.Errorf("PlexRatingKey = %v, want plex-1", input.PlexRatingKey)
	}
}

func TestRepairInputFillsMissingSourceFields(t *testing.T) {
	plan := BuildPlan([]api.PlaylistTrack{{
		ID:            "e-1",
		TrackName:     "Disorder",
		TrackSourceID: strPtr("t-1"),
	}}, Index([]api.LibraryTrack{{ID: "t-1", ReleaseGroupMBID: strPtr("rg-1")}}))

	if len(plan.Repairs) != 1 {
		t.Fatalf("Repairs = %d, want 1", len(plan.Repairs))
	}
	input := plan.Repairs[0].Input
	if input.SourceType != "local" {
		t.Errorf("SourceType = %q, want local", input.SourceType)
	}
	if len(input.AvailableSources) != 1 || input.AvailableSources[0] != "local" {
		t.Errorf("AvailableSources = %v, want [local]", input.AvailableSources)
	}
}

// fakeAPI serves a fixed library and playlist and records the writes.
type fakeAPI struct {
	library   []api.LibraryTrack
	playlists []api.PlaylistSummary
	detail    *api.PlaylistDetail

	added     []string
	positions []int
	removed   []string
}

func (f *fakeAPI) ListLibraryTracks(context.Context) ([]api.LibraryTrack, error) {
	return f.library, nil
}

func (f *fakeAPI) ListPlaylists(context.Context) ([]api.PlaylistSummary, error) {
	return f.playlists, nil
}

func (f *fakeAPI) GetPlaylist(_ context.Context, id string) (*api.PlaylistDetail, error) {
	if f.detail == nil || f.detail.ID != id {
		return nil, errors.New("unknown playlist")
	}
	return f.detail, nil
}

func (f *fakeAPI) AddTracks(_ context.Context, _ string, tracks []api.TrackInput, position *int) ([]api.PlaylistTrack, error) {
	f.added = append(f.added, tracks[0].TrackName)
	if position != nil {
		f.positions = append(f.positions, *position)
	}
	return nil, nil
}

func (f *fakeAPI) RemoveTracks(_ context.Context, _ string, trackIDs []string) error {
	f.removed = append(f.removed, trackIDs...)
	return nil
}

func testFakeAPI() *fakeAPI {
	return &fakeAPI{
		library: []api.LibraryTrack{{ID: "t-1", ReleaseGroupMBID: strPtr("rg-1")}},
		playlists: []api.PlaylistSummary{
			{ID: "pl-1", Name: "Anime", IsOwner: true},
			{ID: "pl-2", Name: "Someone else's", IsOwner: false},
		},
		detail: &api.PlaylistDetail{
			ID:   "pl-1",
			Name: "Anime",
			Tracks: []api.PlaylistTrack{{
				ID:            "e-1",
				Position:      2,
				TrackName:     "Disorder",
				TrackSourceID: strPtr("t-1"),
				CoverURL:      strPtr("broken"),
				SourceType:    "local",
			}},
		},
	}
}

func TestRunnerRepairsInPlace(t *testing.T) {
	fake := testFakeAPI()
	runner := &Runner{API: fake}

	results, err := runner.Run(context.Background(), Target{Name: "Anime"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(results) != 1 || results[0].Repaired != 1 {
		t.Fatalf("results = %+v, want one repaired playlist", results)
	}
	if len(fake.added) != 1 || fake.added[0] != "Disorder" {
		t.Errorf("added = %v, want the repaired entry", fake.added)
	}
	if len(fake.positions) != 1 || fake.positions[0] != 2 {
		t.Errorf("positions = %v, want the original position", fake.positions)
	}
	if len(fake.removed) != 1 || fake.removed[0] != "e-1" {
		t.Errorf("removed = %v, want the old entry", fake.removed)
	}
}

func TestRunnerDryRunWritesNothing(t *testing.T) {
	fake := testFakeAPI()
	runner := &Runner{API: fake, DryRun: true}

	results, err := runner.Run(context.Background(), Target{Name: "Anime"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if results[0].Repaired != 1 {
		t.Errorf("Repaired = %d, want 1 planned", results[0].Repaired)
	}
	if len(fake.added) != 0 || len(fake.removed) != 0 {
		t.Errorf("dry run wrote: added=%v removed=%v", fake.added, fake.removed)
	}
}

func TestRunnerAllSkipsPlaylistsTheCallerDoesNotOwn(t *testing.T) {
	fake := testFakeAPI()
	runner := &Runner{API: fake, All: true, DryRun: true}

	results, err := runner.Run(context.Background(), Target{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(results) != 1 || results[0].PlaylistID != "pl-1" {
		t.Fatalf("results = %+v, want only the owned playlist", results)
	}
}

func TestRunnerRejectsAmbiguousNames(t *testing.T) {
	fake := testFakeAPI()
	fake.playlists = []api.PlaylistSummary{
		{ID: "pl-1", Name: "Anime", IsOwner: true},
		{ID: "pl-3", Name: "anime", IsOwner: true},
	}
	runner := &Runner{API: fake, DryRun: true}

	_, err := runner.Run(context.Background(), Target{Name: "Anime"})
	var ambiguous *AmbiguousPlaylistError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("error = %T %v, want *AmbiguousPlaylistError", err, err)
	}
	if len(ambiguous.Matches) != 2 {
		t.Errorf("matches = %d, want 2", len(ambiguous.Matches))
	}
}

func strPtr(value string) *string { return &value }

func intPtr(value int) *int { return &value }
