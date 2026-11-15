package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUploadImportFilesSendsMultipartFiles(t *testing.T) {
	var (
		method        string
		authorization string
		names         []string
		contents      map[string]string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		authorization = r.Header.Get("Authorization")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("request is not multipart: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		contents = map[string]string{}
		for _, header := range r.MultipartForm.File["files"] {
			names = append(names, header.Filename)
			file, err := header.Open()
			if err != nil {
				t.Errorf("open part: %v", err)
				continue
			}
			data, err := io.ReadAll(file)
			file.Close()
			if err != nil {
				t.Errorf("read part: %v", err)
				continue
			}
			contents[header.Filename] = string(data)
		}

		writeJSON(t, w, ImportJob{
			ID:         "job-1",
			Status:     "completed",
			UploadName: "track.mp3",
			Items:      []ImportItem{{ID: 7, Status: ImportStatusNeedsReview, FolderName: "Loose tracks"}},
		})
	}))
	defer server.Close()

	dir := t.TempDir()
	first := writeTempFile(t, dir, "track.mp3", "audio-bytes")
	second := writeTempFile(t, dir, "cover.jpg", "image-bytes")

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	client.SetToken("tok-1")

	job, err := client.UploadImportFiles(context.Background(), []UploadFile{first, second})
	if err != nil {
		t.Fatalf("UploadImportFiles returned error: %v", err)
	}

	if method != http.MethodPost {
		t.Errorf("method = %q, want POST", method)
	}
	if authorization != "Bearer tok-1" {
		t.Errorf("Authorization = %q, want Bearer tok-1", authorization)
	}
	if len(names) != 2 || names[0] != "track.mp3" || names[1] != "cover.jpg" {
		t.Errorf("part names = %v, want track.mp3 and cover.jpg, all sent as %q", names, "files")
	}
	if contents["track.mp3"] != "audio-bytes" || contents["cover.jpg"] != "image-bytes" {
		t.Errorf("part contents = %v, want the file bytes", contents)
	}
	if job.ID != "job-1" || len(job.Items) != 1 || job.Items[0].ID != 7 {
		t.Errorf("job = %+v, want job-1 with item 7", job)
	}
	if !job.Items[0].NeedsReview() {
		t.Error("item should need review")
	}
}

func TestUploadImportFilesUsesTheBaseNameWhenNoNameIsGiven(t *testing.T) {
	var name string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("request is not multipart: %v", err)
		}
		name = r.MultipartForm.File["files"][0].Filename
		writeJSON(t, w, ImportJob{ID: "job-1"})
	}))
	defer server.Close()

	dir := t.TempDir()
	file := writeTempFile(t, dir, "song.mp3", "bytes")
	file.Name = ""

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if _, err := client.UploadImportFiles(context.Background(), []UploadFile{file}); err != nil {
		t.Fatalf("UploadImportFiles returned error: %v", err)
	}
	if name != "song.mp3" {
		t.Errorf("filename = %q, want the base name song.mp3", name)
	}
}

func TestUploadImportFilesRejectsEmptyInput(t *testing.T) {
	client, err := New("http://localhost:1234", false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if _, err := client.UploadImportFiles(context.Background(), nil); err == nil {
		t.Fatal("UploadImportFiles returned nil error, want a failure")
	}
}

func TestUploadImportFilesReportsMissingFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("the server must not be called when a file is missing")
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	missing := UploadFile{Name: "gone.mp3", Path: filepath.Join(t.TempDir(), "gone.mp3")}
	if _, err := client.UploadImportFiles(context.Background(), []UploadFile{missing}); err == nil {
		t.Fatal("UploadImportFiles returned nil error, want a failure")
	}
}

func TestListImportJobsRequestsEveryUserWhenAsked(t *testing.T) {
	var query string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		writeJSON(t, w, map[string]any{
			"jobs": []ImportJob{{ID: "job-1", Status: "completed", UploadName: "a.mp3"}},
		})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	jobs, err := client.ListImportJobs(context.Background(), true)
	if err != nil {
		t.Fatalf("ListImportJobs returned error: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != "job-1" {
		t.Errorf("jobs = %+v, want job-1", jobs)
	}
	if !strings.Contains(query, "all=true") {
		t.Errorf("query = %q, want all=true", query)
	}

	if _, err := client.ListImportJobs(context.Background(), false); err != nil {
		t.Fatalf("ListImportJobs returned error: %v", err)
	}
	if strings.Contains(query, "all=true") {
		t.Errorf("query = %q, want no all parameter for the user's own jobs", query)
	}
}

func TestGetImportJobReturnsItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/import/jobs/job-9" {
			t.Errorf("path = %q, want /api/v1/import/jobs/job-9", r.URL.Path)
		}
		writeJSON(t, w, ImportJob{
			ID:         "job-9",
			Status:     "completed",
			UploadName: "album",
			Items: []ImportItem{{
				ID:               15,
				Status:           ImportStatusNeedsReview,
				FolderName:       "Loose tracks",
				AlbumTitle:       nil,
				FilesTotal:       1,
				FilesImported:    0,
				Detail:           strPtr("Couldn't work out which album this is."),
				ReleaseGroupMBID: nil,
			}},
		})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	job, err := client.GetImportJob(context.Background(), "job-9")
	if err != nil {
		t.Fatalf("GetImportJob returned error: %v", err)
	}
	if job.UploadName != "album" || len(job.Items) != 1 {
		t.Fatalf("job = %+v, want one item", job)
	}
	if !job.Settled() {
		t.Error("items that reached needs_review are final, so the job should count as settled")
	}
	if (&ImportJob{Items: []ImportItem{{Status: "processing"}}}).Settled() {
		t.Error("a job with a still-running item must not count as settled")
	}
	if job.Items[0].Title() != "" {
		t.Error("Title() should be empty when the server sent null")
	}
	if job.Items[0].Message() == "" {
		t.Error("Message() should expose the server detail")
	}
}

func TestGetImportJobRejectsEmptyID(t *testing.T) {
	client, err := New("http://localhost:1234", false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if _, err := client.GetImportJob(context.Background(), "  "); err == nil {
		t.Fatal("GetImportJob returned nil error, want a failure")
	}
}

func TestImportJobWithNoItemsIsNotSettled(t *testing.T) {
	var job ImportJob

	if job.Settled() {
		t.Error("a job whose items have not been listed yet must not count as settled")
	}
}

func TestMatchImportItemSendsTheReleaseGroup(t *testing.T) {
	const group = "e49c0013-dc74-4fad-b100-ae4d0fe7a1ea"

	var (
		method string
		path   string
		body   map[string]any
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		writeJSON(t, w, ImportItem{
			ID:               15,
			Status:           ImportStatusImported,
			AlbumTitle:       strPtr("T.M.R. LIVE REVOLUTION'15 -Strikes Back XI-"),
			ArtistName:       strPtr("T.M.Revolution"),
			ReleaseGroupMBID: strPtr(group),
			FilesTotal:       1,
			FilesImported:    1,
			Detail:           strPtr("Imported 1, covers 1 of 12 album tracks"),
		})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	item, err := client.MatchImportItem(context.Background(), 15, group)
	if err != nil {
		t.Fatalf("MatchImportItem returned error: %v", err)
	}

	if method != http.MethodPost {
		t.Errorf("method = %q, want POST", method)
	}
	if path != "/api/v1/import/items/15/match" {
		t.Errorf("path = %q, want /api/v1/import/items/15/match", path)
	}
	if body["release_group_mbid"] != group {
		t.Errorf("body = %v, want release_group_mbid %s", body, group)
	}
	if item.Status != ImportStatusImported || item.Title() == "" {
		t.Errorf("item = %+v, want an imported item with a title", item)
	}
}

func TestMatchImportItemRejectsInvalidInput(t *testing.T) {
	client, err := New("http://localhost:1234", false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	cases := []struct {
		name    string
		itemID  int
		groupID string
	}{
		{"zero item", 0, "e49c0013-dc74-4fad-b100-ae4d0fe7a1ea"},
		{"negative item", -1, "e49c0013-dc74-4fad-b100-ae4d0fe7a1ea"},
		{"empty group", 15, "   "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := client.MatchImportItem(context.Background(), tc.itemID, tc.groupID); err == nil {
				t.Fatal("MatchImportItem returned nil error, want a failure")
			}
		})
	}
}

func TestMatchImportItemSurfacesServerErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"detail":"item was already imported"}`))
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = client.MatchImportItem(context.Background(), 13, "e49c0013-dc74-4fad-b100-ae4d0fe7a1ea")
	if err == nil {
		t.Fatal("MatchImportItem returned nil error, want a failure")
	}
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want an *api.Error", err)
	}
	if apiErr.StatusCode != http.StatusConflict || apiErr.Detail != "item was already imported" {
		t.Errorf("error = %+v, want the decoded conflict detail", apiErr)
	}
}

// writeTempFile creates a file and returns it as an upload.
func writeTempFile(t *testing.T, dir, name, content string) UploadFile {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return UploadFile{Name: name, Path: path}
}

func strPtr(value string) *string { return &value }
