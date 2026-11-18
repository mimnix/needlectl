package acquire

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mimmo/needlectl/internal/api"
	"github.com/mimmo/needlectl/internal/mbref"
)

func TestMissingAlbumsGroupsUnplayableEntries(t *testing.T) {
	entries := []api.PlaylistTrack{
		{ID: "e-0", TrackName: "Already there", SourceType: "local"}, // playable
		{ID: "e-1", TrackName: "The Real Me", ArtistName: "The Who", AlbumName: "Quadrophenia"},
		{ID: "e-2", TrackName: "Cut My Hair", ArtistName: "the who", AlbumName: "quadrophenia"},
		{ID: "e-3", TrackName: "Moonglow", ArtistName: "Avantasia", AlbumName: "Moonglow"},
		{ID: "e-4", TrackName: "Loose track", ArtistName: "Nobody"},
	}

	albums := MissingAlbums(entries)
	if len(albums) != 3 {
		t.Fatalf("albums = %d, want 3", len(albums))
	}
	if albums[0].Album != "Quadrophenia" || len(albums[0].Entries) != 2 {
		t.Errorf("first album = %+v, want the two Quadrophenia entries", albums[0])
	}
	if albums[1].Album != "Moonglow" || len(albums[1].Entries) != 1 {
		t.Errorf("second album = %+v", albums[1])
	}
	if albums[2].Album != "" || len(albums[2].Entries) != 1 {
		t.Errorf("album-less group = %+v", albums[2])
	}
}

func TestChooseAlbumRequiresExactTitleAndArtist(t *testing.T) {
	results := []api.SearchResultItem{
		{Kind: "album", Title: "Quadrophenia", Artist: strPtr("The Who"), MusicBrainzID: strPtr("rg-1"), Score: 100},
		{Kind: "album", Title: "Quadrophenia", Artist: strPtr("Some Cover Band"), MusicBrainzID: strPtr("rg-2"), Score: 96},
		{Kind: "album", Title: "Quadrophenia Live", Artist: strPtr("The Who"), MusicBrainzID: strPtr("rg-3"), Score: 90},
		{Kind: "artist", Title: "The Who", MusicBrainzID: strPtr("artist-1")},
	}

	choice, ok := ChooseAlbum(results, "The Who", "Quadrophenia")
	if !ok {
		t.Fatal("ChooseAlbum found no match")
	}
	if choice.MusicBrainzID != "rg-1" || choice.Alternatives != 0 {
		t.Errorf("choice = %+v, want rg-1 with no alternative", choice)
	}
}

func TestChooseAlbumPrefersTheAlbumOverItsSingle(t *testing.T) {
	results := []api.SearchResultItem{
		{Kind: "album", Title: "Holy Diver", Artist: strPtr("Dio"), MusicBrainzID: strPtr("rg-single"), Score: 100, TypeInfo: strPtr("Single")},
		{Kind: "album", Title: "Holy Diver", Artist: strPtr("Dio"), MusicBrainzID: strPtr("rg-album"), Score: 100, TypeInfo: strPtr("Album")},
	}

	choice, ok := ChooseAlbum(results, "Dio", "Holy Diver")
	if !ok {
		t.Fatal("ChooseAlbum found no match")
	}
	if choice.MusicBrainzID != "rg-album" {
		t.Errorf("MusicBrainzID = %q, want the album release group", choice.MusicBrainzID)
	}
}

func TestChooseAlbumTakesTheTopExactMatch(t *testing.T) {
	results := []api.SearchResultItem{
		{Kind: "album", Title: "Ritmo Perfeito", Artist: strPtr("Anitta"), MusicBrainzID: strPtr("rg-album"), Score: 100},
		{Kind: "album", Title: "Ritmo Perfeito", Artist: strPtr("Anitta"), MusicBrainzID: strPtr("rg-single"), Score: 100},
	}

	choice, ok := ChooseAlbum(results, "Anitta", "Ritmo Perfeito")
	if !ok {
		t.Fatal("ChooseAlbum found no match")
	}
	if choice.MusicBrainzID != "rg-album" {
		t.Errorf("MusicBrainzID = %q, want the first ranked hit", choice.MusicBrainzID)
	}
	if choice.Alternatives != 1 {
		t.Errorf("Alternatives = %d, want 1", choice.Alternatives)
	}
}

func TestChooseAlbumRefusesWhenNothingMatches(t *testing.T) {
	results := []api.SearchResultItem{
		{Kind: "album", Title: "Moonglow", Artist: strPtr("Benny Goodman"), MusicBrainzID: strPtr("rg-1"), Score: 70},
		{Kind: "album", Title: "Other", Artist: strPtr("Avantasia"), MusicBrainzID: strPtr("rg-2"), Score: 60},
		{Kind: "album", Title: "Moonglow", Artist: nil, MusicBrainzID: strPtr("rg-3"), Score: 50},
	}

	if _, ok := ChooseAlbum(results, "Avantasia", "Moonglow"); ok {
		t.Error("ChooseAlbum accepted a hit with a mismatched artist")
	}
	if _, ok := ChooseAlbum(results, "Avantasia", "   "); ok {
		t.Error("ChooseAlbum accepted an empty album title")
	}
}

// fakeAPI serves one playlist and resolves albums from a map keyed by album
// title.
type fakeAPI struct {
	playlists []api.PlaylistSummary
	detail    *api.PlaylistDetail
	hits      map[string][]api.SearchResultItem
	requests  []api.AlbumRequest
	trackReqs []api.TrackRequest
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

func (f *fakeAPI) SearchAlbums(_ context.Context, query string, _ int) ([]api.SearchResultItem, error) {
	for album, hits := range f.hits {
		if strings.Contains(strings.ToLower(query), strings.ToLower(album)) {
			return hits, nil
		}
	}
	return nil, nil
}

func (f *fakeAPI) RequestAlbum(_ context.Context, request api.AlbumRequest) (*api.IntakeResponse, error) {
	f.requests = append(f.requests, request)
	return &api.IntakeResponse{Success: true, Status: "queued", Message: "Requested"}, nil
}

func (f *fakeAPI) RequestTrack(_ context.Context, request api.TrackRequest) (*api.TrackIntakeResponse, error) {
	f.trackReqs = append(f.trackReqs, request)
	return &api.TrackIntakeResponse{Status: "queued"}, nil
}

// fakeTracks serves release-group tracklists from a map.
type fakeTracks struct {
	tracks map[string][]mbref.ReleaseTrack
}

func (f *fakeTracks) ReleaseGroupTracks(_ context.Context, releaseGroupMBID string) ([]mbref.ReleaseTrack, error) {
	return f.tracks[releaseGroupMBID], nil
}

func testFakeAPI() *fakeAPI {
	linked := api.PlaylistTrack{ID: "e-0", TrackName: "Already", SourceType: "local"}
	quad := []api.PlaylistTrack{
		{ID: "e-1", TrackName: "The Real Me", ArtistName: "The Who", AlbumName: "Quadrophenia"},
		{ID: "e-2", TrackName: "Cut My Hair", ArtistName: "The Who", AlbumName: "Quadrophenia"},
	}
	missing := []api.PlaylistTrack{
		{ID: "e-3", TrackName: "Moonglow", ArtistName: "Avantasia", AlbumName: "Moonglow"},
		{ID: "e-4", TrackName: "Loose", ArtistName: "Nobody"},
	}
	tracks := append([]api.PlaylistTrack{linked}, quad...)
	tracks = append(tracks, missing...)

	return &fakeAPI{
		playlists: []api.PlaylistSummary{{ID: "pl-1", Name: "Classic Rock", IsOwner: true}},
		detail:    &api.PlaylistDetail{ID: "pl-1", Name: "Classic Rock", Tracks: tracks},
		hits: map[string][]api.SearchResultItem{
			"Quadrophenia": {
				{Kind: "album", Title: "Quadrophenia", Artist: strPtr("The Who"), MusicBrainzID: strPtr("rg-1"), Score: 100},
			},
		},
	}
}

func TestRunnerRequestsMatchedAlbums(t *testing.T) {
	fake := testFakeAPI()
	runner := &Runner{API: fake, Mode: ModeAlbum}

	result, err := runner.Run(context.Background(), Target{Name: "Classic Rock"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Missing != 4 || result.Albums != 3 {
		t.Errorf("missing/albums = %d/%d, want 4/3", result.Missing, result.Albums)
	}
	if result.Requested != 1 || result.Covered != 2 {
		t.Errorf("requested/covered = %d/%d, want 1/2", result.Requested, result.Covered)
	}
	if result.Unmatched != 2 {
		t.Errorf("unmatched = %d, want 2", result.Unmatched)
	}
	if len(fake.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(fake.requests))
	}
	request := fake.requests[0]
	if request.MusicBrainzID != "rg-1" {
		t.Errorf("requested %q, want rg-1", request.MusicBrainzID)
	}
	if request.Album == nil || *request.Album != "Quadrophenia" || request.Artist == nil || *request.Artist != "The Who" {
		t.Errorf("request = %+v", request)
	}
}

func TestRunnerDryRunSendsNothing(t *testing.T) {
	fake := testFakeAPI()
	runner := &Runner{API: fake, Mode: ModeAlbum, DryRun: true}

	result, err := runner.Run(context.Background(), Target{ID: "pl-1"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Requested != 1 {
		t.Errorf("Requested = %d, want 1 planned", result.Requested)
	}
	if len(fake.requests) != 0 {
		t.Errorf("dry run sent %d requests", len(fake.requests))
	}
}

func TestRunnerSkipsAlbumsAlreadyRequested(t *testing.T) {
	fake := testFakeAPI()
	fake.hits["Quadrophenia"] = []api.SearchResultItem{
		{Kind: "album", Title: "Quadrophenia", Artist: strPtr("The Who"), MusicBrainzID: strPtr("rg-1"), Score: 100, Requested: true},
	}
	runner := &Runner{API: fake, Mode: ModeAlbum}

	result, err := runner.Run(context.Background(), Target{ID: "pl-1"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.AlreadyAsked != 1 || result.Requested != 0 {
		t.Errorf("already/requested = %d/%d, want 1/0", result.AlreadyAsked, result.Requested)
	}
	if result.Covered != 2 {
		t.Errorf("Covered = %d, want the album's 2 tracks", result.Covered)
	}
	if len(fake.requests) != 0 {
		t.Errorf("requests = %d, want none", len(fake.requests))
	}
}

func TestRunnerRejectsAmbiguousNames(t *testing.T) {
	fake := testFakeAPI()
	fake.playlists = []api.PlaylistSummary{
		{ID: "pl-1", Name: "Anime", IsOwner: true},
		{ID: "pl-2", Name: "anime", IsOwner: true},
	}
	runner := &Runner{API: fake, DryRun: true}

	_, err := runner.Run(context.Background(), Target{Name: "Anime"})
	var ambiguous *AmbiguousPlaylistError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("error = %T %v, want *AmbiguousPlaylistError", err, err)
	}
}

func strPtr(value string) *string { return &value }

func TestRunnerRequestsMissingRecordings(t *testing.T) {
	fake := testFakeAPI()
	resolver := &fakeTracks{tracks: map[string][]mbref.ReleaseTrack{
		"rg-1": {
			{RecordingMBID: "rec-1", Title: "The Real Me", Artist: "The Who", DurationMS: 201906},
			{RecordingMBID: "rec-2", Title: "Cut My Hair", Artist: "The Who", DurationMS: 227000},
		},
	}}
	runner := &Runner{API: fake, Tracks: resolver}

	result, err := runner.Run(context.Background(), Target{ID: "pl-1"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Mode != ModeTrack {
		t.Errorf("Mode = %v, want per track", result.Mode)
	}
	if result.Requested != 2 || result.Covered != 2 {
		t.Errorf("requested/covered = %d/%d, want 2/2", result.Requested, result.Covered)
	}
	if result.Unmatched != 2 {
		t.Errorf("Unmatched = %d, want the 2 unresolved tracks", result.Unmatched)
	}
	if len(fake.trackReqs) != 2 {
		t.Fatalf("track requests = %d, want 2", len(fake.trackReqs))
	}
	request := fake.trackReqs[0]
	if request.RecordingMBID != "rec-1" {
		t.Errorf("recording = %q, want rec-1", request.RecordingMBID)
	}
	if request.ReleaseGroupMBID == nil || *request.ReleaseGroupMBID != "rg-1" {
		t.Errorf("release group = %v, want rg-1", request.ReleaseGroupMBID)
	}
	if request.AlbumTitle == nil || *request.AlbumTitle != "Quadrophenia" {
		t.Errorf("album = %v, want Quadrophenia", request.AlbumTitle)
	}
	if request.DurationSeconds == nil || *request.DurationSeconds != 202 {
		t.Errorf("duration = %v, want 202", request.DurationSeconds)
	}
	if len(fake.requests) != 0 {
		t.Errorf("album requests = %d, want none in track mode", len(fake.requests))
	}
}

func TestRunnerTrackModeReportsUnmatchedTitles(t *testing.T) {
	fake := testFakeAPI()
	resolver := &fakeTracks{tracks: map[string][]mbref.ReleaseTrack{
		"rg-1": {{RecordingMBID: "rec-1", Title: "The Real Me"}},
	}}
	runner := &Runner{API: fake, Tracks: resolver}

	result, err := runner.Run(context.Background(), Target{ID: "pl-1"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Requested != 1 {
		t.Errorf("Requested = %d, want 1", result.Requested)
	}
	if result.Unmatched != 3 {
		t.Errorf("Unmatched = %d, want 3 (Cut My Hair, Moonglow, album-less)", result.Unmatched)
	}
	if len(fake.trackReqs) != 1 {
		t.Errorf("track requests = %d, want 1", len(fake.trackReqs))
	}
}

func TestRunnerTrackModeSkipsAnAlbumAlreadyRequested(t *testing.T) {
	fake := testFakeAPI()
	fake.hits["Quadrophenia"] = []api.SearchResultItem{
		{Kind: "album", Title: "Quadrophenia", Artist: strPtr("The Who"), MusicBrainzID: strPtr("rg-1"), Score: 100, Requested: true},
	}
	runner := &Runner{API: fake, Tracks: &fakeTracks{}}

	result, err := runner.Run(context.Background(), Target{ID: "pl-1"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.AlreadyAsked != 1 {
		t.Errorf("AlreadyAsked = %d, want 1", result.AlreadyAsked)
	}
	if result.Covered != 2 {
		t.Errorf("Covered = %d, want the album's 2 tracks", result.Covered)
	}
	if len(fake.trackReqs) != 0 {
		t.Errorf("track requests = %d, want none for an album already requested", len(fake.trackReqs))
	}
}

func TestRunnerTrackModeDryRunSendsNothing(t *testing.T) {
	fake := testFakeAPI()
	resolver := &fakeTracks{tracks: map[string][]mbref.ReleaseTrack{
		"rg-1": {
			{RecordingMBID: "rec-1", Title: "The Real Me"},
			{RecordingMBID: "rec-2", Title: "Cut My Hair"},
		},
	}}
	runner := &Runner{API: fake, Tracks: resolver, DryRun: true}

	result, err := runner.Run(context.Background(), Target{ID: "pl-1"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Requested != 2 {
		t.Errorf("Requested = %d, want 2 planned", result.Requested)
	}
	if len(fake.trackReqs) != 0 || len(fake.requests) != 0 {
		t.Errorf("dry run sent track=%d album=%d", len(fake.trackReqs), len(fake.requests))
	}
}
