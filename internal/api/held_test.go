package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListHeldEntriesReadsTheHeldList(t *testing.T) {
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

		writeJSON(t, w, heldListResponse{Entries: []HeldEntry{
			{
				ID:               1,
				Origin:           "download",
				State:            "held",
				AlbumTitle:       strPtr("Strike"),
				ArtistName:       strPtr("Far Out"),
				ReleaseGroupMBID: strPtr("6b2b962f-b2fc-40ac-b418-b1fa161b96cc"),
				CreatedAt:        1794854771136,
				Files: []HeldFile{
					{
						ID:           11,
						OriginalName: "110. Far Out - Strike.mp3",
						ReleaseMBID:  strPtr("ae891a44-138f-4076-b4af-fe29af2f1a5a"),
						TrackNumber:  intPtr(110),
						TrackTitle:   strPtr("Strike"),
						Format:       strPtr("mp3"),
						Source:       "soulseek",
						Reason:       "weak_match",
						ReasonText:   "The files do not match the album closely enough to import them safely.",
						ReasonKind:   "hold",
					},
				},
			},
			{
				ID:     2,
				Origin: "upload",
				State:  "placing",
				Files:  []HeldFile{},
			},
		}})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	client.SetToken("tok-1")

	entries, err := client.ListHeldEntries(context.Background(), false)
	if err != nil {
		t.Fatalf("ListHeldEntries returned error: %v", err)
	}
	if method != http.MethodGet || path != "/api/v3/held" {
		t.Errorf("request = %s %s, want GET /api/v3/held", method, path)
	}
	if rawQuery != "" {
		t.Errorf("query = %q, want none", rawQuery)
	}
	if auth != "Bearer tok-1" {
		t.Errorf("authorization = %q, want the bearer token", auth)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}

	entry := entries[0]
	if entry.ID != 1 || entry.Title() != "Strike" || entry.Artist() != "Far Out" {
		t.Errorf("entry = %+v, want the Far Out single", entry)
	}
	if entry.ReleaseGroup() != "6b2b962f-b2fc-40ac-b418-b1fa161b96cc" {
		t.Errorf("release group = %q", entry.ReleaseGroup())
	}
	if entry.ReasonCode() != "weak_match" {
		t.Errorf("reason code = %q, want weak_match", entry.ReasonCode())
	}
	if len(entry.Files) != 1 || entry.Files[0].OriginalName != "110. Far Out - Strike.mp3" {
		t.Fatalf("files = %+v", entry.Files)
	}
	if entry.Files[0].TrackNumber == nil || *entry.Files[0].TrackNumber != 110 {
		t.Errorf("track number = %v, want 110", entry.Files[0].TrackNumber)
	}

	// Missing optional fields must not fail the decode.
	if entries[1].Title() != "" || entries[1].Artist() != "" || entries[1].ReasonCode() != "" {
		t.Errorf("entry without optional fields = %+v", entries[1])
	}
}

func TestListHeldEntriesPassesAllForAdmins(t *testing.T) {
	var rawQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		writeJSON(t, w, heldListResponse{Entries: []HeldEntry{}})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if _, err := client.ListHeldEntries(context.Background(), true); err != nil {
		t.Fatalf("ListHeldEntries returned error: %v", err)
	}
	if rawQuery != "all=true" {
		t.Errorf("query = %q, want all=true", rawQuery)
	}
}

func TestMatchHeldEntrySendsTheReleaseGroup(t *testing.T) {
	var (
		method   string
		path     string
		auth     string
		body     map[string]any
		rawBytes []byte
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		auth = r.Header.Get("Authorization")
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		rawBytes = data
		if err := json.Unmarshal(data, &body); err != nil {
			t.Errorf("body is not JSON: %v", err)
		}

		writeJSON(t, w, HeldActionResult{
			Imported:     1,
			AlreadyOwned: 0,
			StillHeld:    0,
			Discarded:    0,
			Message:      strPtr("Imported 1 file."),
		})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	client.SetToken("tok-1")

	result, err := client.MatchHeldEntry(context.Background(), 1, " 6b2b962f-b2fc-40ac-b418-b1fa161b96cc ")
	if err != nil {
		t.Fatalf("MatchHeldEntry returned error: %v", err)
	}
	if method != http.MethodPost || path != "/api/v3/held/1/match" {
		t.Errorf("request = %s %s, want POST /api/v3/held/1/match", method, path)
	}
	if auth != "Bearer tok-1" {
		t.Errorf("authorization = %q, want the bearer token", auth)
	}
	if got := body["release_group_mbid"]; got != "6b2b962f-b2fc-40ac-b418-b1fa161b96cc" {
		t.Errorf("release_group_mbid = %v, want the trimmed id", got)
	}
	if _, ok := body["file_ids"]; ok {
		t.Errorf("body = %s, want no file_ids when matching the whole entry", rawBytes)
	}
	if result.Imported != 1 || result.StillHeld != 0 {
		t.Errorf("result = %+v, want one imported file", result)
	}
	if result.Message == nil || *result.Message != "Imported 1 file." {
		t.Errorf("message = %v", result.Message)
	}
}

func TestMatchHeldEntryRejectsInvalidArguments(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		writeJSON(t, w, HeldActionResult{})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if _, err := client.MatchHeldEntry(context.Background(), 0, "6b2b962f-b2fc-40ac-b418-b1fa161b96cc"); err == nil {
		t.Error("MatchHeldEntry with entry id 0 returned no error")
	}
	if _, err := client.MatchHeldEntry(context.Background(), 1, "   "); err == nil {
		t.Error("MatchHeldEntry with a blank release group returned no error")
	}
	if requests != 0 {
		t.Errorf("client sent %d requests, want none", requests)
	}
}

func TestMatchHeldEntrySurfacesServerErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		writeJSON(t, w, map[string]any{
			"error": map[string]any{"code": "NOT_FOUND", "message": "Unknown held entry", "details": nil},
		})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = client.MatchHeldEntry(context.Background(), 99, "6b2b962f-b2fc-40ac-b418-b1fa161b96cc")
	if err == nil {
		t.Fatal("MatchHeldEntry returned no error")
	}
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T, want *api.Error", err)
	}
	if !apiErr.IsNotFound() {
		t.Errorf("status = %d, want 404", apiErr.StatusCode)
	}
}

func intPtr(value int) *int { return &value }
