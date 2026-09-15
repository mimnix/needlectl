package importer

import (
	"testing"

	"github.com/mimmo/needlectl/internal/api"
	"github.com/mimmo/needlectl/internal/matcher"
	"github.com/mimmo/needlectl/internal/playlist"
)

func testRow(track, artist, album string) playlist.Row {
	return playlist.Row{TrackName: track, ArtistName: artist, AlbumName: album}
}

func testIndex(tracks ...matcher.Track) *matcher.Index {
	return matcher.NewIndex(tracks)
}

func linkedEntry(track, artist, album, fileID string) api.PlaylistTrack {
	return api.PlaylistTrack{
		ID:            "entry-" + fileID,
		TrackName:     track,
		ArtistName:    artist,
		AlbumName:     album,
		SourceType:    sourceTypeLocal,
		LibraryFileID: stringPointer(fileID),
	}
}

func TestBuildPlanLinksRowsFoundInLibrary(t *testing.T) {
	rows := []playlist.Row{
		testRow("Disorder", "神崎エルザ", "Elza"),
		testRow("Missing Song", "Nobody", "Nowhere"),
	}
	index := testIndex(matcher.Track{
		ID: "file-1", Title: "Disorder", ArtistName: "神崎エルザ", AlbumTitle: "Elza",
		AlbumID: "rg-1", ArtistID: "ar-1", TrackNumber: 4, DiscNumber: 1, DurationSeconds: 215, Format: "FLAC",
	})

	plan := BuildPlan(rows, nil, index)

	if plan.Matched != 1 || plan.Unmatched != 1 {
		t.Fatalf("matched/unmatched = %d/%d, want 1/1", plan.Matched, plan.Unmatched)
	}
	if len(plan.Adds) != 2 {
		t.Fatalf("len(Adds) = %d, want 2", len(plan.Adds))
	}

	linked := plan.Adds[0]
	if !linked.Linked {
		t.Error("first add should be linked")
	}
	if linked.Track.SourceType != sourceTypeLocal {
		t.Errorf("SourceType = %q, want %q", linked.Track.SourceType, sourceTypeLocal)
	}
	if linked.Track.TrackSourceID == nil || *linked.Track.TrackSourceID != "file-1" {
		t.Errorf("TrackSourceID = %v, want file-1", linked.Track.TrackSourceID)
	}
	if linked.Track.AlbumID == nil || *linked.Track.AlbumID != "rg-1" {
		t.Errorf("AlbumID = %v, want rg-1", linked.Track.AlbumID)
	}

	unlinked := plan.Adds[1]
	if unlinked.Linked {
		t.Error("second add should not be linked")
	}
	if unlinked.Track.SourceType != "" {
		t.Errorf("SourceType = %q, want empty", unlinked.Track.SourceType)
	}
}

func TestBuildPlanIsIdempotentForLinkedEntries(t *testing.T) {
	rows := []playlist.Row{testRow("Disorder", "神崎エルザ", "Elza")}
	existing := []api.PlaylistTrack{linkedEntry("Disorder", "神崎エルザ", "Elza", "file-1")}
	index := testIndex(matcher.Track{ID: "file-1", Title: "Disorder", ArtistName: "神崎エルザ", AlbumTitle: "Elza"})

	plan := BuildPlan(rows, existing, index)

	if len(plan.Adds) != 0 {
		t.Errorf("len(Adds) = %d, want 0", len(plan.Adds))
	}
	if len(plan.Refreshes) != 0 {
		t.Errorf("len(Refreshes) = %d, want 0", len(plan.Refreshes))
	}
	if plan.Unchanged != 1 {
		t.Errorf("Unchanged = %d, want 1", plan.Unchanged)
	}
}

func TestBuildPlanRefreshesUnlinkedEntriesNowInLibrary(t *testing.T) {
	rows := []playlist.Row{testRow("Disorder", "神崎エルザ", "Elza")}
	existing := []api.PlaylistTrack{{
		ID: "entry-1", Position: 3, TrackName: "Disorder", ArtistName: "神崎エルザ", AlbumName: "Elza",
	}}
	index := testIndex(matcher.Track{ID: "file-1", Title: "Disorder", ArtistName: "神崎エルザ", AlbumTitle: "Elza"})

	plan := BuildPlan(rows, existing, index)

	if len(plan.Adds) != 0 {
		t.Fatalf("len(Adds) = %d, want 0", len(plan.Adds))
	}
	if len(plan.Refreshes) != 1 {
		t.Fatalf("len(Refreshes) = %d, want 1", len(plan.Refreshes))
	}
	if plan.Refreshes[0].Existing.ID != "entry-1" {
		t.Errorf("refresh targets %q, want entry-1", plan.Refreshes[0].Existing.ID)
	}
	if plan.Refreshes[0].Track.TrackSourceID == nil || *plan.Refreshes[0].Track.TrackSourceID != "file-1" {
		t.Errorf("refresh TrackSourceID = %v, want file-1", plan.Refreshes[0].Track.TrackSourceID)
	}
}

func TestBuildPlanKeepsUnlinkedEntryWhenStillMissing(t *testing.T) {
	rows := []playlist.Row{testRow("Missing Song", "Nobody", "Nowhere")}
	existing := []api.PlaylistTrack{{
		ID: "entry-1", TrackName: "Missing Song", ArtistName: "Nobody", AlbumName: "Nowhere",
	}}

	plan := BuildPlan(rows, existing, testIndex())

	if len(plan.Refreshes) != 0 {
		t.Errorf("len(Refreshes) = %d, want 0", len(plan.Refreshes))
	}
	if plan.Unchanged != 1 {
		t.Errorf("Unchanged = %d, want 1", plan.Unchanged)
	}
}

func TestBuildPlanPreservesDuplicateRows(t *testing.T) {
	rows := []playlist.Row{
		testRow("Song", "Artist", "Album"),
		testRow("Song", "Artist", "Album"),
	}

	emptyPlan := BuildPlan(rows, nil, testIndex())
	if len(emptyPlan.Adds) != 2 {
		t.Errorf("len(Adds) with no existing entries = %d, want 2", len(emptyPlan.Adds))
	}

	existing := []api.PlaylistTrack{linkedEntry("Song", "Artist", "Album", "file-1")}
	plan := BuildPlan(rows, existing, testIndex())
	if len(plan.Adds) != 1 {
		t.Errorf("len(Adds) with one existing entry = %d, want 1", len(plan.Adds))
	}
	if plan.Unchanged != 1 {
		t.Errorf("Unchanged = %d, want 1", plan.Unchanged)
	}
}

func TestPlanChanged(t *testing.T) {
	if (Plan{}).Changed() {
		t.Error("empty plan should not report a change")
	}
	if !(Plan{Adds: []Add{{}}}).Changed() {
		t.Error("plan with an add should report a change")
	}
	if !(Plan{Refreshes: []Refresh{{}}}).Changed() {
		t.Error("plan with a refresh should report a change")
	}
}
