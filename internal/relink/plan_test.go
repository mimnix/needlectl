package relink

import (
	"testing"

	"github.com/mimmo/needlectl/internal/api"
	"github.com/mimmo/needlectl/internal/link"
	"github.com/mimmo/needlectl/internal/matcher"
)

func testIndex(tracks ...matcher.Track) *matcher.Index {
	return matcher.NewIndex(tracks)
}

func unplayableEntry(id, track, artist, album string, position int) api.PlaylistTrack {
	return api.PlaylistTrack{
		ID: id, Position: position,
		TrackName: track, ArtistName: artist, AlbumName: album,
	}
}

func playableEntry(id, track, artist, album string, position int) api.PlaylistTrack {
	fileID := "file-" + id
	return api.PlaylistTrack{
		ID: id, Position: position,
		TrackName: track, ArtistName: artist, AlbumName: album,
		SourceType:    link.SourceTypeLocal,
		TrackSourceID: &fileID,
		LibraryFileID: &fileID,
	}
}

func TestBuildPlanSelectsOnlyUnplayableEntriesFoundInTheLibrary(t *testing.T) {
	tracks := []api.PlaylistTrack{
		unplayableEntry("entry-1", "Disorder", "神崎エルザ", "Elza", 0),
		unplayableEntry("entry-2", "Missing Song", "Nobody", "Nowhere", 1),
	}
	index := testIndex(matcher.Track{
		ID: "file-1", Title: "Disorder", ArtistName: "神崎エルザ", AlbumTitle: "Elza",
		AlbumID: "rg-1", ArtistID: "ar-1", TrackNumber: 4, DurationSeconds: 215, Format: "FLAC",
		CoverAvailable: true,
	})

	plan := BuildPlan(tracks, index)

	if plan.Total != 2 {
		t.Errorf("Total = %d, want 2", plan.Total)
	}
	if plan.Playable != 0 {
		t.Errorf("Playable = %d, want 0", plan.Playable)
	}
	if plan.Unmatched != 1 {
		t.Errorf("Unmatched = %d, want 1", plan.Unmatched)
	}
	if len(plan.Relinks) != 1 {
		t.Fatalf("len(Relinks) = %d, want 1", len(plan.Relinks))
	}

	item := plan.Relinks[0]
	if item.Existing.ID != "entry-1" {
		t.Errorf("relinked entry = %q, want entry-1", item.Existing.ID)
	}
	if item.Library.ID != "file-1" {
		t.Errorf("library match = %q, want file-1", item.Library.ID)
	}
	if item.Track.TrackSourceID == nil || *item.Track.TrackSourceID != "file-1" {
		t.Errorf("TrackSourceID = %v, want file-1", item.Track.TrackSourceID)
	}
	if item.Track.SourceType != link.SourceTypeLocal {
		t.Errorf("SourceType = %q, want %q", item.Track.SourceType, link.SourceTypeLocal)
	}
	if item.Track.CoverURL == nil || *item.Track.CoverURL != "/api/v1/covers/release-group/rg-1?size=250" {
		t.Errorf("CoverURL = %v, want the album artwork link", item.Track.CoverURL)
	}
}

func TestBuildPlanLeavesPlayableEntriesUntouched(t *testing.T) {
	tracks := []api.PlaylistTrack{
		playableEntry("entry-1", "Disorder", "神崎エルザ", "Elza", 0),
	}
	index := testIndex(matcher.Track{ID: "file-1", Title: "Disorder", ArtistName: "神崎エルザ", AlbumTitle: "Elza"})

	plan := BuildPlan(tracks, index)

	if len(plan.Relinks) != 0 {
		t.Errorf("a playable entry must not be relinked, got %d relinks", len(plan.Relinks))
	}
	if plan.Playable != 1 {
		t.Errorf("Playable = %d, want 1", plan.Playable)
	}
}

func TestBuildPlanTreatsAnyExistingSourceAsPlayable(t *testing.T) {
	track := unplayableEntry("entry-1", "Disorder", "神崎エルザ", "Elza", 0)
	track.SourceType = "navidrome"
	track.TrackSourceID = strPtr("nd-1")
	index := testIndex(matcher.Track{ID: "file-1", Title: "Disorder", ArtistName: "神崎エルザ", AlbumTitle: "Elza"})

	plan := BuildPlan([]api.PlaylistTrack{track}, index)

	if plan.Playable != 1 || len(plan.Relinks) != 0 {
		t.Errorf("playable/relinks = %d/%d, want 1/0", plan.Playable, len(plan.Relinks))
	}
}

func TestBuildPlanKeepsTheEntrySpellingInTheLinkPayload(t *testing.T) {
	tracks := []api.PlaylistTrack{
		unplayableEntry("entry-1", "Disorder", "神崎エルザ", "Elza (Deluxe)", 0),
	}
	index := testIndex(matcher.Track{ID: "file-1", Title: "Disorder", ArtistName: "神崎エルザ", AlbumTitle: "Elza"})

	plan := BuildPlan(tracks, index)

	if len(plan.Relinks) != 1 {
		t.Fatalf("len(Relinks) = %d, want 1", len(plan.Relinks))
	}
	got := plan.Relinks[0].Track
	if got.TrackName != "Disorder" || got.ArtistName != "神崎エルザ" || got.AlbumName != "Elza (Deluxe)" {
		t.Errorf("payload must keep the playlist spelling, got %+v", got)
	}
}

func TestBuildPlanCountsEmptyPlaylist(t *testing.T) {
	plan := BuildPlan(nil, testIndex())

	if plan.Total != 0 || plan.Playable != 0 || plan.Unmatched != 0 || plan.Changed() {
		t.Errorf("plan for an empty playlist = %+v, want all zero", plan)
	}
}

func TestPlanChanged(t *testing.T) {
	if (Plan{}).Changed() {
		t.Error("empty plan should not report a change")
	}
	if !(Plan{Relinks: []Relink{{}}}).Changed() {
		t.Error("plan with a relink should report a change")
	}
}

func strPtr(value string) *string { return &value }
