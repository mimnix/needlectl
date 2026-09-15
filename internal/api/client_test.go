package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestNewNormalizesBaseURL(t *testing.T) {
	cases := map[string]string{
		"localhost:8688":           "http://localhost:8688",
		"127.0.0.1:8688":           "http://127.0.0.1:8688",
		"http://localhost:8688/":   "http://localhost:8688",
		"https://music.test/":      "https://music.test",
		"http://host/dn/":          "http://host/dn",
		"music.example.test":       "https://music.example.test",
		"music.example.test:8688/": "https://music.example.test:8688",
	}
	for input, want := range cases {
		client, err := New(input, false)
		if err != nil {
			t.Fatalf("New(%q) returned error: %v", input, err)
		}
		if client.BaseURL() != want {
			t.Errorf("New(%q).BaseURL() = %q, want %q", input, client.BaseURL(), want)
		}
	}
}

func TestNewRejectsEmptyBaseURL(t *testing.T) {
	if _, err := New("   ", false); err == nil {
		t.Fatal("New returned nil error, want a failure")
	}
}

func TestNewRejectsUnsafeBaseURLs(t *testing.T) {
	cases := []string{
		"ftp://music.example.test",
		"file:///etc/passwd",
		"https://user:secret@music.example.test",
	}
	for _, input := range cases {
		if _, err := New(input, false); err == nil {
			t.Errorf("New(%q) returned nil error, want a failure", input)
		}
	}
}

func TestLoginStoresTokenForSubsequentRequests(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/login":
			writeJSON(t, w, AuthResponse{Token: "tok-123", User: User{ID: "u1", DisplayName: "Admin", Role: "admin"}})
		case "/api/v1/auth/me":
			authorization = r.Header.Get("Authorization")
			writeJSON(t, w, User{ID: "u1", DisplayName: "Admin", Role: "admin"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	user, err := client.Login(context.Background(), "admin", "secret")
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}
	if user.DisplayName != "Admin" {
		t.Errorf("DisplayName = %q, want Admin", user.DisplayName)
	}

	if _, err := client.Me(context.Background()); err != nil {
		t.Fatalf("Me returned error: %v", err)
	}
	if authorization != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want Bearer tok-123", authorization)
	}
}

func TestListLibraryTracksFollowsPagination(t *testing.T) {
	firstPage := make([]LibraryTrack, 200)
	for i := range firstPage {
		firstPage[i] = LibraryTrack{ID: fmt.Sprintf("id-%d", i)}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		switch {
		case offset == 0:
			writeJSON(t, w, libraryTracksResponse{Items: firstPage, Total: 201, Offset: 0, Limit: 200})
		case offset == 200:
			writeJSON(t, w, libraryTracksResponse{Items: []LibraryTrack{{ID: "id-200"}}, Total: 201, Offset: 200, Limit: 200})
		default:
			writeJSON(t, w, libraryTracksResponse{Total: 201, Offset: offset})
		}
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	tracks, err := client.ListLibraryTracks(context.Background())
	if err != nil {
		t.Fatalf("ListLibraryTracks returned error: %v", err)
	}
	if len(tracks) != 201 {
		t.Fatalf("len(tracks) = %d, want 201", len(tracks))
	}
	if tracks[200].ID != "id-200" {
		t.Errorf("last track = %q, want id-200", tracks[200].ID)
	}
}

func TestListLibraryTracksRejectsInconsistentPagination(t *testing.T) {
	fullPage := make([]LibraryTrack, 200)
	for i := range fullPage {
		fullPage[i] = LibraryTrack{ID: fmt.Sprintf("id-%d", i)}
	}

	// The server ignores the offset and keeps returning the same first page.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, libraryTracksResponse{Items: fullPage, Total: 400, Offset: 0, Limit: 200})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if _, err := client.ListLibraryTracks(context.Background()); err == nil {
		t.Fatal("ListLibraryTracks returned nil error, want a pagination failure")
	}
}

func TestErrorCarriesStatusCodeAndDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		writeJSON(t, w, map[string]string{"detail": "Invalid username or password"})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = client.Login(context.Background(), "admin", "wrong")
	if err == nil {
		t.Fatal("Login returned nil error, want a failure")
	}

	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not an *api.Error", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, want 401", apiErr.StatusCode)
	}
	if !apiErr.IsUnauthorized() {
		t.Error("IsUnauthorized() = false, want true")
	}
	if apiErr.Detail != "Invalid username or password" {
		t.Errorf("Detail = %q", apiErr.Detail)
	}
}

func TestAddTracksSendsPositionAndSource(t *testing.T) {
	var received addTracksRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/playlists/pl-1/tracks" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		writeJSON(t, w, addTracksResponse{})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	position := 2
	fileID := "file-1"
	_, err = client.AddTracks(context.Background(), "pl-1", []TrackInput{{
		TrackName: "Disorder", ArtistName: "神崎エルザ", AlbumName: "Elza",
		SourceType: "local", TrackSourceID: &fileID,
	}}, &position)
	if err != nil {
		t.Fatalf("AddTracks returned error: %v", err)
	}

	if received.Position == nil || *received.Position != 2 {
		t.Errorf("Position = %v, want 2", received.Position)
	}
	if len(received.Tracks) != 1 || received.Tracks[0].SourceType != "local" {
		t.Errorf("Tracks = %+v", received.Tracks)
	}
	if received.Tracks[0].TrackSourceID == nil || *received.Tracks[0].TrackSourceID != "file-1" {
		t.Errorf("TrackSourceID = %v, want file-1", received.Tracks[0].TrackSourceID)
	}
}

func TestResolveSourcesDecodesMap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, resolveSourcesResponse{Sources: map[string][]string{"t1": {"local"}}})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	sources, err := client.ResolveSources(context.Background(), "pl-1")
	if err != nil {
		t.Fatalf("ResolveSources returned error: %v", err)
	}
	if len(sources["t1"]) != 1 || sources["t1"][0] != "local" {
		t.Errorf("sources = %+v, want t1:[local]", sources)
	}
}

func TestPlaylistTrackLinked(t *testing.T) {
	fileID := "file-1"
	if !(PlaylistTrack{LibraryFileID: &fileID}).Linked() {
		t.Error("track with a library file id should be linked")
	}
	if (PlaylistTrack{}).Linked() {
		t.Error("empty track should not be linked")
	}
	if !(PlaylistTrack{SourceType: "navidrome"}).Linked() {
		t.Error("track with a source type should be linked")
	}
	if (PlaylistTrack{AvailableSources: []string{"local"}}).Linked() {
		t.Error("candidate sources alone must not count as linked")
	}
}

func TestPlaylistLifecycleRequests(t *testing.T) {
	var removeBody removeTracksRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/playlists":
			writeJSON(t, w, playlistListResponse{Playlists: []PlaylistSummary{{ID: "pl-1", Name: "Anime"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/playlists":
			writeJSON(t, w, PlaylistDetail{ID: "pl-2", Name: "New"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/playlists/pl-1":
			writeJSON(t, w, PlaylistDetail{ID: "pl-1", Name: "Anime"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/playlists/pl-1/tracks/remove":
			if err := json.NewDecoder(r.Body).Decode(&removeBody); err != nil {
				t.Errorf("decode remove body: %v", err)
			}
			writeJSON(t, w, map[string]string{"status": "ok"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	ctx := context.Background()

	playlists, err := client.ListPlaylists(ctx)
	if err != nil || len(playlists) != 1 || playlists[0].Name != "Anime" {
		t.Fatalf("ListPlaylists = %+v, %v", playlists, err)
	}

	created, err := client.CreatePlaylist(ctx, "New")
	if err != nil || created.ID != "pl-2" {
		t.Fatalf("CreatePlaylist = %+v, %v", created, err)
	}

	detail, err := client.GetPlaylist(ctx, "pl-1")
	if err != nil || detail.Name != "Anime" {
		t.Fatalf("GetPlaylist = %+v, %v", detail, err)
	}

	if err := client.RemoveTracks(ctx, "pl-1", []string{"entry-1"}); err != nil {
		t.Fatalf("RemoveTracks returned error: %v", err)
	}
	if len(removeBody.TrackIDs) != 1 || removeBody.TrackIDs[0] != "entry-1" {
		t.Errorf("remove body = %+v", removeBody)
	}
}

func TestRemoveTracksWithNoIDsSkipsTheRequest(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		writeJSON(t, w, map[string]string{"status": "ok"})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := client.RemoveTracks(context.Background(), "pl-1", nil); err != nil {
		t.Fatalf("RemoveTracks returned error: %v", err)
	}
	if called {
		t.Error("RemoveTracks sent a request for an empty id list")
	}
}

func TestErrorFallsBackToRawBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("upstream exploded"))
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = client.ListPlaylists(context.Background())
	if err == nil || !strings.Contains(err.Error(), "upstream exploded") {
		t.Fatalf("error = %v, want it to include the raw body", err)
	}
}

func TestExtractDetailHandlesValidationArray(t *testing.T) {
	body := []byte(`{"detail":[{"loc":["body"],"msg":"field required"}]}`)
	got := extractDetail(body)
	if !strings.Contains(got, "field required") {
		t.Errorf("extractDetail = %q, want it to include the validation message", got)
	}
}

func TestListLibraryAlbumsPaginates(t *testing.T) {
	firstPage := make([]LibraryAlbum, 50)
	for i := range firstPage {
		firstPage[i] = LibraryAlbum{ID: fmt.Sprintf("album-%d", i)}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		switch page {
		case 1:
			writeJSON(t, w, libraryAlbumsResponse{Items: firstPage, Total: 51})
		case 2:
			writeJSON(t, w, libraryAlbumsResponse{Items: []LibraryAlbum{{ID: "album-50"}}, Total: 51})
		default:
			writeJSON(t, w, libraryAlbumsResponse{Total: 51})
		}
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	albums, err := client.ListLibraryAlbums(context.Background(), "elza")
	if err != nil {
		t.Fatalf("ListLibraryAlbums returned error: %v", err)
	}
	if len(albums) != 51 || albums[50].ID != "album-50" {
		t.Fatalf("albums = %d, last = %q", len(albums), albums[len(albums)-1].ID)
	}
}

func TestListLibraryAlbumsFailsWhenTooManyMatch(t *testing.T) {
	fullPage := make([]LibraryAlbum, 50)
	for i := range fullPage {
		fullPage[i] = LibraryAlbum{ID: fmt.Sprintf("album-%d", i)}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, libraryAlbumsResponse{Items: fullPage, Total: 1_000_000})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if _, err := client.ListLibraryAlbums(context.Background(), "a"); err == nil {
		t.Fatal("ListLibraryAlbums returned nil error, want a truncation failure")
	}
}

func TestGetLibraryAlbum(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/library/albums/album-1" {
			http.NotFound(w, r)
			return
		}
		format := "FLAC"
		year := 2018
		writeJSON(t, w, LibraryAlbum{ID: "album-1", Title: "Elza", ArtistName: "神崎エルザ", Format: &format, Year: &year, TrackCount: 12})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	album, err := client.GetLibraryAlbum(context.Background(), "album-1")
	if err != nil {
		t.Fatalf("GetLibraryAlbum returned error: %v", err)
	}
	if album.Title != "Elza" || album.TrackCount != 12 {
		t.Errorf("album = %+v", album)
	}
}

func TestDeleteAlbumSendsOptions(t *testing.T) {
	var method, path, deleteFiles, stopWanted string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		deleteFiles = r.URL.Query().Get("delete_files")
		stopWanted = r.URL.Query().Get("stop_wanted")
		writeJSON(t, w, CatalogRemovalResponse{Success: true, ID: "album-1", RemovedTrackIDs: []string{"t1", "t2"}})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	removal, err := client.DeleteAlbum(context.Background(), "album-1", true, false)
	if err != nil {
		t.Fatalf("DeleteAlbum returned error: %v", err)
	}
	if method != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", method)
	}
	if path != "/api/v1/library/album/album-1" {
		t.Errorf("path = %q", path)
	}
	if deleteFiles != "true" || stopWanted != "false" {
		t.Errorf("delete_files=%q stop_wanted=%q, want true/false", deleteFiles, stopWanted)
	}
	if len(removal.RemovedTrackIDs) != 2 {
		t.Errorf("removed tracks = %v, want 2", removal.RemovedTrackIDs)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
