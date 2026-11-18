package link

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mimnix/needlectl/internal/api"
	"github.com/mimnix/needlectl/internal/matcher"
)

func TestIndexBuildsMatcherFromLibraryTracks(t *testing.T) {
	index := Index([]api.LibraryTrack{{
		ID: "file-1", Title: "Disorder", ArtistName: "神崎エルザ", AlbumTitle: "Elza",
		AlbumID: "rg-1", ArtistID: "ar-1",
	}})

	if index.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", index.Len())
	}
	match, ok := index.Match("Disorder", "神崎エルザ", "Elza")
	if !ok || match.ID != "file-1" {
		t.Errorf("Match = %+v, %v, want file-1", match, ok)
	}
	if match.AlbumID != "rg-1" || match.ArtistID != "ar-1" {
		t.Errorf("ids not carried over: %+v", match)
	}
}

func TestInputBuildsLocalLinkPayload(t *testing.T) {
	input := Input("Disorder", "神崎エルザ", "Elza", matcher.Track{
		ID: "file-1", AlbumID: "rg-1", ArtistID: "ar-1",
		TrackNumber: 4, DiscNumber: 1, DurationSeconds: 215.4, Format: "FLAC",
	})

	if input.TrackName != "Disorder" || input.ArtistName != "神崎エルザ" || input.AlbumName != "Elza" {
		t.Errorf("entry metadata not preserved: %+v", input)
	}
	if input.SourceType != SourceTypeLocal {
		t.Errorf("SourceType = %q, want %q", input.SourceType, SourceTypeLocal)
	}
	if len(input.AvailableSources) != 1 || input.AvailableSources[0] != SourceTypeLocal {
		t.Errorf("AvailableSources = %v, want [%s]", input.AvailableSources, SourceTypeLocal)
	}

	assertStringPointer(t, "AlbumID", input.AlbumID, "rg-1")
	assertStringPointer(t, "ArtistID", input.ArtistID, "ar-1")
	assertStringPointer(t, "TrackSourceID", input.TrackSourceID, "file-1")
	assertStringPointer(t, "Format", input.Format, "FLAC")
	assertIntPointer(t, "TrackNumber", input.TrackNumber, 4)
	assertIntPointer(t, "DiscNumber", input.DiscNumber, 1)
	assertIntPointer(t, "Duration", input.Duration, 215)
}

func TestInputOmitsEmptyLibraryFields(t *testing.T) {
	input := Input("Song", "Artist", "Album", matcher.Track{ID: "file-1"})

	for name, value := range map[string]*string{
		"AlbumID":  input.AlbumID,
		"ArtistID": input.ArtistID,
		"Format":   input.Format,
		"CoverURL": input.CoverURL,
	} {
		if value != nil {
			t.Errorf("%s = %q, want nil", name, *value)
		}
	}
	if input.Duration != nil {
		t.Errorf("Duration = %d, want nil", *input.Duration)
	}
	if input.TrackNumber != nil || input.DiscNumber != nil {
		t.Error("track/disc number should be nil when not positive")
	}
}

func TestCoverURLUsesTheReleaseGroupNotTheLocalAlbum(t *testing.T) {
	library := matcher.Track{
		ID: "file-1",
		// The local album id is NOT what the covers endpoint takes.
		AlbumID:        "8a1f-local-album",
		ReleaseGroupID: "cd98137e-68eb-57c4-9c76-7e875d3a0f03",
	}

	if got, want := CoverURL(library), "/api/v3/covers/release-group/cd98137e-68eb-57c4-9c76-7e875d3a0f03?size=250"; got != want {
		t.Errorf("CoverURL = %q, want %q", got, want)
	}
}

func TestCoverURLIsEmptyWhenThereIsNoReleaseGroup(t *testing.T) {
	cases := map[string]matcher.Track{
		"track without a release group": {ID: "file-1", AlbumID: "local-1"},
		"empty track":                   {},
	}
	for name, library := range cases {
		if got := CoverURL(library); got != "" {
			t.Errorf("%s: CoverURL = %q, want empty", name, got)
		}
	}
}

func TestCoverURLDoesNotDependOnLocalCoverAvailability(t *testing.T) {
	// cover_available reports locally scanned art; the server also fetches art
	// for the release group, so a false flag must not suppress the link.
	library := matcher.Track{ID: "file-1", ReleaseGroupID: "rg-1", CoverAvailable: false}

	if got, want := CoverURL(library), "/api/v3/covers/release-group/rg-1?size=250"; got != want {
		t.Errorf("CoverURL = %q, want %q", got, want)
	}
}

func TestInputCarriesTheCoverURLOfTheReleaseGroup(t *testing.T) {
	input := Input("Disorder", "神崎エルザ", "Elza", matcher.Track{
		ID: "file-1", AlbumID: "local-1", ReleaseGroupID: "rg-1",
	})

	assertStringPointer(t, "CoverURL", input.CoverURL, "/api/v3/covers/release-group/rg-1?size=250")
	assertStringPointer(t, "AlbumID", input.AlbumID, "local-1")
}

func TestIndexCarriesCoverAvailabilityAndReleaseGroup(t *testing.T) {
	index := Index([]api.LibraryTrack{
		{ID: "file-1", Title: "Disorder", ArtistName: "神崎エルザ", AlbumTitle: "Elza", AlbumID: "local-1", ReleaseGroupMBID: strPtr("rg-1"), CoverAvailable: true},
		{ID: "file-2", Title: "Rising Hope", ArtistName: "LiSA", AlbumTitle: "Launcher", AlbumID: "local-2"},
	})

	withCover, ok := index.Match("Disorder", "神崎エルザ", "Elza")
	if !ok {
		t.Fatal("Disorder did not match")
	}
	withoutCover, ok := index.Match("Rising Hope", "LiSA", "Launcher")
	if !ok {
		t.Fatal("Rising Hope did not match")
	}

	if !withCover.CoverAvailable {
		t.Error("CoverAvailable was lost for file-1")
	}
	if withoutCover.CoverAvailable {
		t.Error("CoverAvailable was invented for file-2")
	}
	if withCover.ReleaseGroupID != "rg-1" {
		t.Errorf("ReleaseGroupID = %q, want rg-1", withCover.ReleaseGroupID)
	}
	if got := CoverURL(withCover); got == "" {
		t.Error("file-1 should produce a cover URL")
	}
	if got := CoverURL(withoutCover); got != "" {
		t.Errorf("file-2 has no release group, got %q", got)
	}
}

// fakeReplacer records the calls Replace makes.
type fakeReplacer struct {
	calls     []string
	added     []api.TrackInput
	position  *int
	removed   []string
	addErr    error
	removeErr error
}

func (f *fakeReplacer) AddTracks(_ context.Context, _ string, tracks []api.TrackInput, position *int) ([]api.PlaylistTrack, error) {
	f.calls = append(f.calls, "add")
	f.added = append(f.added, tracks...)
	f.position = position
	return nil, f.addErr
}

func (f *fakeReplacer) RemoveTracks(_ context.Context, _ string, trackIDs []string) error {
	f.calls = append(f.calls, "remove")
	f.removed = append(f.removed, trackIDs...)
	return f.removeErr
}

func TestReplaceInsertsAtPositionBeforeRemoving(t *testing.T) {
	fake := &fakeReplacer{}
	existing := api.PlaylistTrack{ID: "entry-1", Position: 3}
	input := Input("Disorder", "神崎エルザ", "Elza", matcher.Track{ID: "file-1"})

	if err := Replace(context.Background(), fake, "pl-1", existing, input); err != nil {
		t.Fatalf("Replace returned error: %v", err)
	}

	if len(fake.calls) != 2 || fake.calls[0] != "add" || fake.calls[1] != "remove" {
		t.Fatalf("calls = %v, want [add remove]", fake.calls)
	}
	if fake.position == nil || *fake.position != 3 {
		t.Errorf("position = %v, want 3", fake.position)
	}
	if len(fake.added) != 1 || fake.added[0].TrackSourceID == nil || *fake.added[0].TrackSourceID != "file-1" {
		t.Errorf("added = %+v", fake.added)
	}
	if len(fake.removed) != 1 || fake.removed[0] != "entry-1" {
		t.Errorf("removed = %v, want [entry-1]", fake.removed)
	}
}

func TestReplaceKeepsOriginalWhenInsertFails(t *testing.T) {
	fake := &fakeReplacer{addErr: errors.New("boom")}

	err := Replace(context.Background(), fake, "pl-1",
		api.PlaylistTrack{ID: "entry-1"}, Input("Song", "Artist", "Album", matcher.Track{ID: "file-1"}))

	if err == nil {
		t.Fatal("Replace returned nil error, want a failure")
	}
	if len(fake.calls) != 1 || fake.calls[0] != "add" {
		t.Errorf("calls = %v, want only the failed add", fake.calls)
	}
}

func TestReplaceReportsRemoveFailure(t *testing.T) {
	fake := &fakeReplacer{removeErr: errors.New("boom")}

	err := Replace(context.Background(), fake, "pl-1",
		api.PlaylistTrack{ID: "entry-1"}, Input("Song", "Artist", "Album", matcher.Track{ID: "file-1"}))

	if err == nil {
		t.Fatal("Replace returned nil error, want a failure")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error %q should warn about a possible duplicate", err)
	}
}

func assertStringPointer(t *testing.T, name string, got *string, want string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s = nil, want %q", name, want)
	}
	if *got != want {
		t.Errorf("%s = %q, want %q", name, *got, want)
	}
}

func assertIntPointer(t *testing.T, name string, got *int, want int) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s = nil, want %d", name, want)
	}
	if *got != want {
		t.Errorf("%s = %d, want %d", name, *got, want)
	}
}

func strPtr(value string) *string { return &value }
