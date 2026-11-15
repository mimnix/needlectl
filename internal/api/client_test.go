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
	var (
		authorization string
		loginBody     map[string]string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/auth/login":
			if err := json.NewDecoder(r.Body).Decode(&loginBody); err != nil {
				t.Errorf("decode login body: %v", err)
			}
			writeJSON(t, w, AuthResponse{Token: "tok-123", User: User{ID: "u1", DisplayName: "Admin", Role: "admin"}})
		case "/api/v3/me":
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
	if loginBody["transport"] != "bearer" {
		t.Errorf("login transport = %q, want bearer (cookies would return no token)", loginBody["transport"])
	}
	if loginBody["username"] != "admin" || loginBody["password"] != "secret" {
		t.Errorf("login body = %+v, want the credentials", loginBody)
	}
}

func TestListLibraryTracksFollowsCursorPagination(t *testing.T) {
	firstPage := make([]LibraryTrack, 200)
	for i := range firstPage {
		firstPage[i] = LibraryTrack{ID: fmt.Sprintf("id-%d", i), RowRevision: int64(i + 1)}
	}

	var afterParams []string
	var limits []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/library/tracks" {
			t.Errorf("path = %q, want /api/v3/library/tracks", r.URL.Path)
		}
		afterParams = append(afterParams, r.URL.Query().Get("after"))
		limits = append(limits, r.URL.Query().Get("limit"))
		if len(afterParams) == 1 {
			writeJSON(t, w, libraryTracksResponse{Items: firstPage, Next: strPtr("cursor-2"), Total: 201})
			return
		}
		writeJSON(t, w, libraryTracksResponse{Items: []LibraryTrack{{ID: "id-200", RowRevision: 201}}, Total: 201})
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
	if tracks[200].RowRevision != 201 {
		t.Errorf("last RowRevision = %d, want 201", tracks[200].RowRevision)
	}
	if len(afterParams) != 2 {
		t.Fatalf("requests = %d, want 2 (stop when next is null)", len(afterParams))
	}
	if afterParams[0] != "" {
		t.Errorf("first request after = %q, want no cursor", afterParams[0])
	}
	if afterParams[1] != "cursor-2" {
		t.Errorf("second request after = %q, want cursor-2", afterParams[1])
	}
	for i, limit := range limits {
		if limit != "200" {
			t.Errorf("request %d limit = %q, want 200", i+1, limit)
		}
	}
}

func TestListLibraryTracksRejectsRepeatedCursor(t *testing.T) {
	fullPage := make([]LibraryTrack, 200)
	for i := range fullPage {
		fullPage[i] = LibraryTrack{ID: fmt.Sprintf("id-%d", i)}
	}

	// The server keeps handing out a cursor that points at the same page.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, libraryTracksResponse{Items: fullPage, Next: strPtr("cursor-2"), Total: 400})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = client.ListLibraryTracks(context.Background())
	if err == nil {
		t.Fatal("ListLibraryTracks returned nil error, want a cursor failure")
	}
	if !strings.Contains(err.Error(), "repeated page cursor") {
		t.Errorf("error = %q, want it to mention the repeated page cursor", err)
	}
}

func TestListLibraryTracksRejectsACursorWithoutItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, libraryTracksResponse{Items: nil, Next: strPtr("cursor-2"), Total: 400})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = client.ListLibraryTracks(context.Background())
	if err == nil {
		t.Fatal("ListLibraryTracks returned nil error, want a cursor failure")
	}
	if !strings.Contains(err.Error(), "no items") {
		t.Errorf("error = %q, want it to mention the empty page", err)
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
		if r.URL.Path != "/api/v3/playlists/pl-1/tracks" {
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
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/playlists":
			writeJSON(t, w, playlistListResponse{Playlists: []PlaylistSummary{{ID: "pl-1", Name: "Anime"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v3/playlists":
			writeJSON(t, w, PlaylistDetail{ID: "pl-2", Name: "New"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/playlists/pl-1":
			writeJSON(t, w, PlaylistDetail{ID: "pl-1", Name: "Anime"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v3/playlists/pl-1/tracks/remove":
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

func TestListLibraryAlbumsFollowsCursorPagination(t *testing.T) {
	firstPage := make([]LibraryAlbum, 50)
	for i := range firstPage {
		firstPage[i] = LibraryAlbum{ID: fmt.Sprintf("album-%d", i)}
	}

	var afterParams []string
	var limits []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/library/albums" {
			t.Errorf("path = %q, want /api/v3/library/albums", r.URL.Path)
		}
		if got := r.URL.Query().Get("q"); got != "elza" {
			t.Errorf("q = %q, want elza", got)
		}
		afterParams = append(afterParams, r.URL.Query().Get("after"))
		limits = append(limits, r.URL.Query().Get("limit"))
		if len(afterParams) == 1 {
			writeJSON(t, w, libraryAlbumsResponse{Items: firstPage, Next: strPtr("album-cursor-2"), Total: 51})
			return
		}
		writeJSON(t, w, libraryAlbumsResponse{Items: []LibraryAlbum{{ID: "album-50"}}, Total: 51})
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
	if len(afterParams) != 2 {
		t.Fatalf("requests = %d, want 2 (stop when next is null)", len(afterParams))
	}
	if afterParams[0] != "" {
		t.Errorf("first request after = %q, want no cursor", afterParams[0])
	}
	if afterParams[1] != "album-cursor-2" {
		t.Errorf("second request after = %q, want album-cursor-2", afterParams[1])
	}
	for i, limit := range limits {
		if limit != "50" {
			t.Errorf("request %d limit = %q, want 50", i+1, limit)
		}
	}
}

func TestListLibraryAlbumsRejectsRepeatedCursor(t *testing.T) {
	fullPage := make([]LibraryAlbum, 50)
	for i := range fullPage {
		fullPage[i] = LibraryAlbum{ID: fmt.Sprintf("album-%d", i)}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, libraryAlbumsResponse{Items: fullPage, Next: strPtr("album-cursor-2"), Total: 400})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = client.ListLibraryAlbums(context.Background(), "")
	if err == nil {
		t.Fatal("ListLibraryAlbums returned nil error, want a cursor failure")
	}
	if !strings.Contains(err.Error(), "repeated page cursor") {
		t.Errorf("error = %q, want it to mention the repeated page cursor", err)
	}
}

func TestListLibraryAlbumsFailsWhenTooManyMatch(t *testing.T) {
	fullPage := make([]LibraryAlbum, 50)
	for i := range fullPage {
		fullPage[i] = LibraryAlbum{ID: fmt.Sprintf("album-%d", i)}
	}

	// Every page advertises a fresh cursor, so the client must stop itself
	// after maxAlbumPages instead of trusting the server's total.
	page := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		page++
		writeJSON(t, w, libraryAlbumsResponse{
			Items: fullPage,
			Next:  strPtr("album-cursor-" + strconv.Itoa(page)),
			Total: 1_000_000,
		})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = client.ListLibraryAlbums(context.Background(), "a")
	if err == nil {
		t.Fatal("ListLibraryAlbums returned nil error, want a truncation failure")
	}
	if !strings.Contains(err.Error(), "too many albums") {
		t.Errorf("error = %q, want the too-many-albums failure", err)
	}
	if page != maxAlbumPages {
		t.Errorf("server was asked for %d pages, want %d", page, maxAlbumPages)
	}
}

func TestGetLibraryAlbum(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/library/albums/album-1" {
			http.NotFound(w, r)
			return
		}
		format := "FLAC"
		year := 2018
		writeJSON(t, w, LibraryAlbum{
			ID: "album-1", Title: "Elza", ArtistName: "神崎エルザ", Format: &format, Year: &year, TrackCount: 12,
			RowRevision: 11, ListingRevision: 5,
		})
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
	if album.RowRevision != 11 || album.ListingRevision != 5 {
		t.Errorf("revisions = %d/%d, want 11/5", album.RowRevision, album.ListingRevision)
	}
}

func TestDeleteAlbumReadsTheRevisionThenDeletes(t *testing.T) {
	var (
		calls                   []string
		deleteFiles, stopWant   string
		expectedRev, listingRev string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/library/albums/album-1" {
			t.Errorf("path = %q, want /api/v3/library/albums/album-1", r.URL.Path)
		}
		calls = append(calls, r.Method)
		switch r.Method {
		case http.MethodGet:
			writeJSON(t, w, LibraryAlbum{ID: "album-1", RowRevision: 11, ListingRevision: 5})
		case http.MethodDelete:
			deleteFiles = r.URL.Query().Get("delete_files")
			stopWant = r.URL.Query().Get("stop_wanted")
			expectedRev = r.URL.Query().Get("expected_revision")
			listingRev = r.URL.Query().Get("expected_listing_revision")
			writeJSON(t, w, RemovalResponse{ID: "album-1", RemovedTrackIDs: []string{"t1", "t2"}})
		default:
			http.NotFound(w, r)
		}
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
	if len(calls) != 2 || calls[0] != http.MethodGet || calls[1] != http.MethodDelete {
		t.Fatalf("calls = %v, want GET then DELETE", calls)
	}
	if expectedRev != "11" || listingRev != "5" {
		t.Errorf("expected_revision=%q expected_listing_revision=%q, want 11/5", expectedRev, listingRev)
	}
	if deleteFiles != "true" || stopWant != "false" {
		t.Errorf("delete_files=%q stop_wanted=%q, want true/false", deleteFiles, stopWant)
	}
	if removal.ID != "album-1" || len(removal.RemovedTrackIDs) != 2 {
		t.Errorf("removal = %+v, want album-1 with 2 tracks", removal)
	}
}

func TestSearchLibraryTracksSendsQuery(t *testing.T) {
	var gotQuery, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("q")
		if after := r.URL.Query().Get("after"); after != "" {
			t.Errorf("first page after = %q, want no cursor", after)
		}
		writeJSON(t, w, libraryTracksResponse{
			Items: []LibraryTrack{{ID: "track-1", Title: "Disorder"}},
			Total: 1,
		})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	tracks, err := client.SearchLibraryTracks(context.Background(), "Disorder")
	if err != nil {
		t.Fatalf("SearchLibraryTracks returned error: %v", err)
	}
	if gotPath != "/api/v3/library/tracks" {
		t.Errorf("path = %q, want /api/v3/library/tracks", gotPath)
	}
	if gotQuery != "Disorder" {
		t.Errorf("q = %q, want Disorder", gotQuery)
	}
	if len(tracks) != 1 || tracks[0].ID != "track-1" {
		t.Errorf("tracks = %+v", tracks)
	}
}

func TestDeleteTrackSendsTheKnownRevision(t *testing.T) {
	var (
		methods []string
		path    string
		body    RemovalResponse
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		path = r.URL.Path
		if got := r.URL.Query().Get("expected_revision"); got != "9" {
			t.Errorf("expected_revision = %q, want 9", got)
		}
		if got := r.URL.Query().Get("delete_file"); got != "true" {
			t.Errorf("delete_file = %q, want true", got)
		}
		body = RemovalResponse{ID: "track-1", RemovedTrackIDs: []string{"track-1"}}
		writeJSON(t, w, body)
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	removal, err := client.DeleteTrack(context.Background(), "track-1", 9)
	if err != nil {
		t.Fatalf("DeleteTrack returned error: %v", err)
	}
	if len(methods) != 1 || methods[0] != http.MethodDelete {
		t.Errorf("calls = %v, want a single DELETE", methods)
	}
	if path != "/api/v3/library/tracks/track-1" {
		t.Errorf("path = %q", path)
	}
	if removal.ID != "track-1" || len(removal.RemovedTrackIDs) != 1 {
		t.Errorf("removal = %+v, want track-1", removal)
	}
}

func TestDeleteTrackLooksUpTheRevisionWhenMissing(t *testing.T) {
	var (
		methods []string
		paths   []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		paths = append(paths, r.URL.Path)
		switch r.Method {
		case http.MethodGet:
			if got := r.URL.Query().Get("limit"); got != "200" {
				t.Errorf("limit = %q, want 200", got)
			}
			writeJSON(t, w, libraryTracksResponse{
				Items: []LibraryTrack{
					{ID: "track-0", RowRevision: 3},
					{ID: "track-1", RowRevision: 42},
				},
				Total: 2,
			})
		case http.MethodDelete:
			if got := r.URL.Query().Get("expected_revision"); got != "42" {
				t.Errorf("expected_revision = %q, want the listed row revision 42", got)
			}
			writeJSON(t, w, RemovalResponse{ID: "track-1", RemovedTrackIDs: []string{"track-1"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if _, err := client.DeleteTrack(context.Background(), "track-1", 0); err != nil {
		t.Fatalf("DeleteTrack returned error: %v", err)
	}
	if len(methods) != 2 || methods[0] != http.MethodGet || methods[1] != http.MethodDelete {
		t.Fatalf("calls = %v, want GET then DELETE", methods)
	}
	if paths[0] != "/api/v3/library/tracks" || paths[1] != "/api/v3/library/tracks/track-1" {
		t.Errorf("paths = %v", paths)
	}
}

func TestDeleteTrackRejectsATrackThatIsNotInTheLibrary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			t.Error("the client must not delete a track it could not find")
		}
		writeJSON(t, w, libraryTracksResponse{Items: []LibraryTrack{{ID: "other", RowRevision: 1}}, Total: 1})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if _, err := client.DeleteTrack(context.Background(), "missing", -1); err == nil {
		t.Fatal("DeleteTrack returned nil error, want a lookup failure")
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
