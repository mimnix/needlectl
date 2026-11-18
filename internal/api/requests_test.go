package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchAlbumsReadsTheBucket(t *testing.T) {
	var (
		method   string
		path     string
		rawQuery string
		auth     string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		rawQuery = r.URL.RawQuery
		auth = r.Header.Get("Authorization")
		writeJSON(t, w, searchBucketResponse{Results: []SearchResultItem{
			{
				Kind:          "album",
				Title:         "Quadrophenia",
				Artist:        strPtr("The Who"),
				MusicBrainzID: strPtr("9b191575-fa1f-3531-bff5-121782318972"),
				Score:         100,
				InLibrary:     false,
				Year:          intPtr(1973),
			},
			{Kind: "artist", Title: "The Who", MusicBrainzID: strPtr("artist-1")},
		}})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	client.SetToken("tok-1")

	results, err := client.SearchAlbums(context.Background(), "Quadrophenia The Who", 5)
	if err != nil {
		t.Fatalf("SearchAlbums returned error: %v", err)
	}
	if method != http.MethodGet || path != "/api/v3/search/albums" {
		t.Errorf("request = %s %s, want GET /api/v3/search/albums", method, path)
	}
	if rawQuery != "limit=5&q=Quadrophenia+The+Who" {
		t.Errorf("query = %q", rawQuery)
	}
	if auth != "Bearer tok-1" {
		t.Errorf("authorization = %q", auth)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	hit := results[0]
	if hit.Title != "Quadrophenia" || hit.Artist == nil || *hit.Artist != "The Who" {
		t.Errorf("hit = %+v", hit)
	}
	if hit.MusicBrainzID == nil || *hit.MusicBrainzID != "9b191575-fa1f-3531-bff5-121782318972" {
		t.Errorf("musicbrainz id = %v", hit.MusicBrainzID)
	}
	if hit.Year == nil || *hit.Year != 1973 {
		t.Errorf("year = %v", hit.Year)
	}
}

func TestSearchAlbumsRejectsABlankQuery(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		writeJSON(t, w, searchBucketResponse{})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if _, err := client.SearchAlbums(context.Background(), "   ", 10); err == nil {
		t.Error("SearchAlbums with a blank query returned no error")
	}
	if requests != 0 {
		t.Errorf("client sent %d requests, want none", requests)
	}
}

func TestRequestAlbumSendsTheReleaseGroup(t *testing.T) {
	var (
		method string
		path   string
		body   map[string]any
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		if err := json.Unmarshal(data, &body); err != nil {
			t.Errorf("body is not JSON: %v", err)
		}
		writeJSON(t, w, IntakeResponse{
			Success:       true,
			Message:       "Requested",
			MusicBrainzID: "9b191575-fa1f-3531-bff5-121782318972",
			Status:        "queued",
			TaskID:        strPtr("task-1"),
		})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	response, err := client.RequestAlbum(context.Background(), AlbumRequest{
		MusicBrainzID: " 9b191575-fa1f-3531-bff5-121782318972 ",
		Album:         strPtr("Quadrophenia"),
		Artist:        strPtr("The Who"),
	})
	if err != nil {
		t.Fatalf("RequestAlbum returned error: %v", err)
	}
	if method != http.MethodPost || path != "/api/v3/requests/albums" {
		t.Errorf("request = %s %s, want POST /api/v3/requests/albums", method, path)
	}
	if got := body["musicbrainz_id"]; got != "9b191575-fa1f-3531-bff5-121782318972" {
		t.Errorf("musicbrainz_id = %v, want the trimmed id", got)
	}
	if got := body["album"]; got != "Quadrophenia" {
		t.Errorf("album = %v", got)
	}
	if got := body["artist"]; got != "The Who" {
		t.Errorf("artist = %v", got)
	}
	if !response.Success || response.Status != "queued" || response.TaskID == nil || *response.TaskID != "task-1" {
		t.Errorf("response = %+v", response)
	}
}

func TestRequestAlbumRejectsABlankReleaseGroup(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		writeJSON(t, w, IntakeResponse{})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if _, err := client.RequestAlbum(context.Background(), AlbumRequest{MusicBrainzID: "  "}); err == nil {
		t.Error("RequestAlbum with a blank id returned no error")
	}
	if requests != 0 {
		t.Errorf("client sent %d requests, want none", requests)
	}
}

func TestRequestTrackSendsTheRecording(t *testing.T) {
	var (
		method string
		path   string
		body   map[string]any
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		if err := json.Unmarshal(data, &body); err != nil {
			t.Errorf("body is not JSON: %v", err)
		}
		writeJSON(t, w, TrackIntakeResponse{Status: "queued", TaskID: strPtr("task-1")})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	duration := 202
	response, err := client.RequestTrack(context.Background(), TrackRequest{
		RecordingMBID:    " rec-1 ",
		ArtistName:       "The Who",
		TrackTitle:       "The Real Me",
		AlbumTitle:       strPtr("Quadrophenia"),
		ReleaseGroupMBID: strPtr("rg-1"),
		DurationSeconds:  &duration,
	})
	if err != nil {
		t.Fatalf("RequestTrack returned error: %v", err)
	}
	if method != http.MethodPost || path != "/api/v3/requests/tracks" {
		t.Errorf("request = %s %s, want POST /api/v3/requests/tracks", method, path)
	}
	if got := body["recording_mbid"]; got != "rec-1" {
		t.Errorf("recording_mbid = %v, want the trimmed id", got)
	}
	if got := body["artist_name"]; got != "The Who" {
		t.Errorf("artist_name = %v", got)
	}
	if got := body["track_title"]; got != "The Real Me" {
		t.Errorf("track_title = %v", got)
	}
	if got := body["release_group_mbid"]; got != "rg-1" {
		t.Errorf("release_group_mbid = %v", got)
	}
	if response.Status != "queued" || response.TaskID == nil || *response.TaskID != "task-1" {
		t.Errorf("response = %+v", response)
	}
}

func TestRequestTrackRejectsIncompleteArguments(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		writeJSON(t, w, TrackIntakeResponse{})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	for name, request := range map[string]TrackRequest{
		"no recording": {ArtistName: "The Who", TrackTitle: "The Real Me"},
		"no artist":    {RecordingMBID: "rec-1", TrackTitle: "The Real Me"},
		"no title":     {RecordingMBID: "rec-1", ArtistName: "The Who"},
	} {
		if _, err := client.RequestTrack(context.Background(), request); err == nil {
			t.Errorf("%s: RequestTrack returned no error", name)
		}
	}
	if requests != 0 {
		t.Errorf("client sent %d requests, want none", requests)
	}
}
