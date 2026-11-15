package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListAlbumTracksReadsTheAlbumTracksEndpoint(t *testing.T) {
	var (
		gotPath string
		offsets []string
		limits  []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		offsets = append(offsets, r.URL.Query().Get("offset"))
		limits = append(limits, r.URL.Query().Get("limit"))
		writeJSON(t, w, albumTracksResponse{
			Items:  []LibraryTrack{{ID: "t-1", Title: "Disorder", RowRevision: 4}},
			Total:  1,
			Offset: 0,
			Limit:  libraryPageSize,
		})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	tracks, err := client.ListAlbumTracks(context.Background(), "a-1")
	if err != nil {
		t.Fatalf("ListAlbumTracks returned error: %v", err)
	}
	if gotPath != "/api/v3/library/albums/a-1/tracks" {
		t.Errorf("path = %q", gotPath)
	}
	if len(offsets) != 1 || offsets[0] != "0" || limits[0] != "200" {
		t.Errorf("offset/limit = %v/%v, want 0/200", offsets, limits)
	}
	if len(tracks) != 1 || tracks[0].ID != "t-1" {
		t.Errorf("tracks = %+v", tracks)
	}
	if tracks[0].RowRevision != 4 {
		t.Errorf("RowRevision = %d, want 4", tracks[0].RowRevision)
	}
}

func TestGetTagEditorAndFieldLookup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/library/management/tracks/t-1/tag-editor" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"local_track_id": "t-1",
			"local_album_id": "a-1",
			"settings_revision": "sr-1",
			"policy_revision": "pr-1",
			"accepted_identity": true,
			"fields": [
				{"field_name": "album", "scope": "album", "cardinality": "string", "current_value": "Elza"},
				{"field_name": "title", "scope": "track", "cardinality": "string", "current_value": "Disorder"},
				{"field_name": "artist", "scope": "track", "cardinality": "ordered_strings", "current_value": ["神崎エルザ"]}
			]
		}`))
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	editor, err := client.GetTagEditor(context.Background(), "t-1")
	if err != nil {
		t.Fatalf("GetTagEditor returned error: %v", err)
	}
	if !editor.AcceptedIdentity || editor.SettingsRevision != "sr-1" || editor.PolicyRevision != "pr-1" {
		t.Errorf("editor = %+v", editor)
	}

	album, ok := editor.Field("album")
	if !ok {
		t.Fatal("album field not found")
	}
	if album.Scope != "album" || album.StringValue() != "Elza" {
		t.Errorf("album field = %+v", album)
	}

	artist, ok := editor.Field("artist")
	if !ok {
		t.Fatal("artist field not found")
	}
	if artist.StringValue() != "" {
		t.Errorf("non-string field StringValue = %q, want empty", artist.StringValue())
	}
	if _, ok := editor.Field("missing"); ok {
		t.Error("Field(missing) reported a field")
	}
}

func TestCreateTagEditPreviewSendsTheEdit(t *testing.T) {
	var path string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		writeJSON(t, w, PreviewRef{JobID: "job-1", PreviewToken: "tok-1"})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	ref, err := client.CreateTagEditPreview(context.Background(), TagEditPreviewRequest{
		LocalTrackID:             "t-1",
		Mode:                     TagEditModeSaveOverride,
		ExpectedSettingsRevision: "sr-1",
		ExpectedPolicyRevision:   "pr-1",
		Fields:                   []TagEditField{{FieldName: "album", Value: "Elza (Deluxe)"}},
		IdempotencyKey:           "key-1",
	})
	if err != nil {
		t.Fatalf("CreateTagEditPreview returned error: %v", err)
	}
	if path != "/api/v3/library/management/tag-edit-previews" {
		t.Errorf("path = %q", path)
	}
	if ref.JobID != "job-1" || ref.PreviewToken != "tok-1" {
		t.Errorf("ref = %+v", ref)
	}
	if body["mode"] != TagEditModeSaveOverride || body["local_track_id"] != "t-1" {
		t.Errorf("body = %+v", body)
	}
	if body["expected_settings_revision"] != "sr-1" || body["expected_policy_revision"] != "pr-1" {
		t.Errorf("revisions not sent: %+v", body)
	}
	if body["idempotency_key"] != "key-1" {
		t.Errorf("idempotency_key = %v", body["idempotency_key"])
	}
	fields, ok := body["fields"].([]any)
	if !ok || len(fields) != 1 {
		t.Fatalf("fields = %#v", body["fields"])
	}
	field := fields[0].(map[string]any)
	if field["field_name"] != "album" || field["value"] != "Elza (Deluxe)" {
		t.Errorf("field = %+v", field)
	}
}

func TestCreateTagEditPreviewOmitsTheValueWhenResetting(t *testing.T) {
	// A reset carries no value; the field must not be serialised as null.
	payload, err := json.Marshal(TagEditPreviewRequest{
		LocalTrackID: "t-1",
		Mode:         TagEditModeResetCanonical,
		Fields:       []TagEditField{{FieldName: "album"}},
	})
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if strings.Contains(string(payload), `"value"`) {
		t.Errorf("reset payload %s should omit the value", payload)
	}
}

func TestGetManagementPreviewKeepsTheRawRevision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/library/management/previews/job-1" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"job_id": "job-1", "state": "ready", "stale": false, "expired": false,
			"ready_for_confirmation": true, "operation_row_revision": 7
		}`))
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	preview, err := client.GetManagementPreview(context.Background(), "job-1")
	if err != nil {
		t.Fatalf("GetManagementPreview returned error: %v", err)
	}
	if !preview.ReadyForConfirmation || preview.State != "ready" {
		t.Errorf("preview = %+v", preview)
	}
	if preview.JobID != "job-1" {
		t.Errorf("JobID = %q, want job-1", preview.JobID)
	}
	if string(preview.OperationRowRevision) != "7" {
		t.Errorf("operation_row_revision = %q, want the raw 7", preview.OperationRowRevision)
	}
}

func TestApplyManagementPreviewEchoesTheRevision(t *testing.T) {
	var path string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		writeJSON(t, w, OperationRef{ID: "op-1"})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	operation, err := client.ApplyManagementPreview(context.Background(), "job-1", ApplyPreviewRequest{
		PreviewToken:                 "tok-1",
		ExpectedOperationRowRevision: json.RawMessage("7"),
		IdempotencyKey:               "key-2",
		Confirmation:                 true,
	})
	if err != nil {
		t.Fatalf("ApplyManagementPreview returned error: %v", err)
	}
	if path != "/api/v3/library/management/previews/job-1/apply" {
		t.Errorf("path = %q", path)
	}
	if operation.ID != "op-1" {
		t.Errorf("operation = %+v", operation)
	}
	if body["preview_token"] != "tok-1" || body["confirmation"] != true {
		t.Errorf("body = %+v", body)
	}
	if body["expected_operation_row_revision"] != float64(7) {
		t.Errorf("expected_operation_row_revision = %#v, want 7", body["expected_operation_row_revision"])
	}
}

func TestDiscardManagementPreviewSendsTheRevision(t *testing.T) {
	var (
		method string
		path   string
		body   map[string]any
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode discard body: %v", err)
		}
		writeJSON(t, w, map[string]string{"status": "ok"})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if err := client.DiscardManagementPreview(context.Background(), "job-1", json.RawMessage("7")); err != nil {
		t.Fatalf("DiscardManagementPreview returned error: %v", err)
	}
	if method != http.MethodPost || path != "/api/v3/library/management/previews/job-1/discard" {
		t.Errorf("%s %s", method, path)
	}
	if body["expected_operation_row_revision"] != float64(7) {
		t.Errorf("expected_operation_row_revision = %#v, want 7", body["expected_operation_row_revision"])
	}
}

func TestDiscardManagementPreviewDefaultsTheRevisionToZero(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode discard body: %v", err)
		}
		writeJSON(t, w, map[string]string{"status": "ok"})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if err := client.DiscardManagementPreview(context.Background(), "job-1", nil); err != nil {
		t.Fatalf("DiscardManagementPreview returned error: %v", err)
	}
	if body["expected_operation_row_revision"] != float64(0) {
		t.Errorf("expected_operation_row_revision = %#v, want 0", body["expected_operation_row_revision"])
	}
}

func TestNewIdempotencyKeyIsAUniqueUUID(t *testing.T) {
	first, err := NewIdempotencyKey()
	if err != nil {
		t.Fatalf("NewIdempotencyKey returned error: %v", err)
	}
	second, err := NewIdempotencyKey()
	if err != nil {
		t.Fatalf("NewIdempotencyKey returned error: %v", err)
	}

	if first == second {
		t.Error("two keys were identical")
	}
	if len(first) != 36 || strings.Count(first, "-") != 4 {
		t.Errorf("key %q is not a UUID", first)
	}
	if first[14] != '4' {
		t.Errorf("key %q is not version 4", first)
	}
}
