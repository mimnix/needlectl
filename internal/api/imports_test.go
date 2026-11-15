package api

import (
	"context"
	"encoding/json"
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
		path          string
		authorization string
		names         []string
		contents      map[string]string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
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
	if path != "/api/v3/import/uploads" {
		t.Errorf("path = %q, want /api/v3/import/uploads", path)
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
	var (
		query string
		path  string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		path = r.URL.Path
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
	if path != "/api/v3/import/jobs" {
		t.Errorf("path = %q, want /api/v3/import/jobs", path)
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

func TestGetImportJobFindsTheJobInTheRecentListing(t *testing.T) {
	var requestPath, requestQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		requestQuery = r.URL.RawQuery
		writeJSON(t, w, map[string]any{
			"jobs": []ImportJob{
				{ID: "job-8", Status: "completed", UploadName: "other"},
				{
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
						ReasonCode:       strPtr("no_release_group_match"),
						ReleaseGroupMBID: nil,
					}},
				},
			},
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
	if requestPath != "/api/v3/import/jobs" {
		t.Errorf("path = %q, want /api/v3/import/jobs", requestPath)
	}
	if requestQuery != "" {
		t.Errorf("query = %q, want no all parameter", requestQuery)
	}
	if job.UploadName != "album" || len(job.Items) != 1 {
		t.Fatalf("job = %+v, want one item", job)
	}
	if job.Items[0].ReasonCode == nil || *job.Items[0].ReasonCode != "no_release_group_match" {
		t.Errorf("ReasonCode = %v", job.Items[0].ReasonCode)
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

func TestGetImportJobRejectsAJobOutsideTheRecentListing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{
			"jobs": []ImportJob{{ID: "job-1", Status: "completed"}},
		})
	}))
	defer server.Close()

	client, err := New(server.URL, false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = client.GetImportJob(context.Background(), "job-9")
	if err == nil {
		t.Fatal("GetImportJob returned nil error, want a failure")
	}
	if !strings.Contains(err.Error(), "not in the recent jobs list") {
		t.Errorf("error = %q, want it to point at the recent jobs list", err)
	}
}

func TestGetImportJobRejectsAnEmptyID(t *testing.T) {
	client, err := New("http://localhost:1234", false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	for _, id := range []string{"", "   ", "\t\n"} {
		if _, err := client.GetImportJob(context.Background(), id); err == nil {
			t.Errorf("GetImportJob(%q) returned nil error, want a failure", id)
		}
	}
}

func TestImportJobWithNoItemsIsNotSettled(t *testing.T) {
	var job ImportJob

	if job.Settled() {
		t.Error("a job whose items have not been listed yet must not count as settled")
	}
}

func TestImportItemSettled(t *testing.T) {
	cases := map[string]bool{
		ImportStatusProcessing:  false,
		ImportStatusImported:    true,
		ImportStatusSkipped:     true,
		ImportStatusNeedsReview: true,
		ImportStatusFailed:      true,
		ImportStatusDiscarded:   true,
		"unknown_status":        false,
	}
	for status, want := range cases {
		if got := (ImportItem{Status: status}).Settled(); got != want {
			t.Errorf("Settled(%q) = %v, want %v", status, got, want)
		}
	}
}

func TestImportItemReadsReasonFields(t *testing.T) {
	raw := `{"id":15,"status":"failed","reason_code":"no_audio_files","reason_action":"pick_another_folder"}`
	var item ImportItem
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if item.ReasonCode == nil || *item.ReasonCode != "no_audio_files" {
		t.Errorf("ReasonCode = %v", item.ReasonCode)
	}
	if item.ReasonAction == nil || *item.ReasonAction != "pick_another_folder" {
		t.Errorf("ReasonAction = %v", item.ReasonAction)
	}

	var withoutReason ImportItem
	if err := json.Unmarshal([]byte(`{"id":16,"status":"imported","reason_code":null,"reason_action":null}`), &withoutReason); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if withoutReason.ReasonCode != nil || withoutReason.ReasonAction != nil {
		t.Errorf("reason fields = %v/%v, want nil", withoutReason.ReasonCode, withoutReason.ReasonAction)
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
