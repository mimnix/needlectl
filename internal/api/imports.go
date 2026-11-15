package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// UploadFile is one file sent to the drop import endpoint.
type UploadFile struct {
	// Name is the file name DroppedNeedle sees. It derives the imported track
	// name from it, so it should stay meaningful.
	Name string
	// Path is the local file the bytes are read from.
	Path string
}

// UploadImportFiles sends files to DroppedNeedle's drop import and returns the
// job created for them.
//
// The endpoint accepts no MusicBrainz reference: DroppedNeedle always identifies
// the files itself. Entries it cannot resolve end in the "needs_review" state,
// which v3 exposes only in its web UI.
func (c *Client) UploadImportFiles(ctx context.Context, files []UploadFile) (*ImportJob, error) {
	if len(files) == 0 {
		return nil, errors.New("no files to upload")
	}
	if err := validateUploadFiles(files); err != nil {
		return nil, err
	}

	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	contentType := multipartWriter.FormDataContentType()

	go func() {
		writer.CloseWithError(writeUploadBody(multipartWriter, files))
	}()

	data, err := c.doReader(ctx, http.MethodPost, "/api/v3/import/uploads", contentType, reader)
	if err != nil {
		return nil, err
	}

	var job ImportJob
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, fmt.Errorf("decode import job: %w", err)
	}
	if job.ID == "" {
		return nil, errors.New("the server did not return an import job")
	}
	return &job, nil
}

// ListImportJobs returns the drop import jobs of the authenticated user. When
// all is true it returns the jobs of every user instead.
func (c *Client) ListImportJobs(ctx context.Context, all bool) ([]ImportJob, error) {
	path := "/api/v3/import/jobs"
	if all {
		path += "?" + url.Values{"all": {"true"}}.Encode()
	}

	data, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var resp importJobsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode import jobs: %w", err)
	}
	return resp.Jobs, nil
}

// GetImportJob returns one import job together with its items.
//
// DroppedNeedle v3 has no single-job endpoint: the job listing carries the full
// items, so the job is looked up there.
func (c *Client) GetImportJob(ctx context.Context, jobID string) (*ImportJob, error) {
	trimmed := strings.TrimSpace(jobID)
	if trimmed == "" {
		return nil, errors.New("import job id is empty")
	}

	jobs, err := c.ListImportJobs(ctx, false)
	if err != nil {
		return nil, err
	}
	for i := range jobs {
		if jobs[i].ID == trimmed {
			return &jobs[i], nil
		}
	}
	return nil, fmt.Errorf("import job %s is not in the recent jobs list", trimmed)
}

// validateUploadFiles fails before any request is made when a file cannot be
// read, so a typo cannot turn into a half-sent upload.
func validateUploadFiles(files []UploadFile) error {
	for _, file := range files {
		info, err := os.Stat(file.Path)
		if err != nil {
			return fmt.Errorf("cannot read upload file: %w", err)
		}
		if info.IsDir() {
			return fmt.Errorf("%s is a directory, expected a file", file.Path)
		}
	}
	return nil
}

// writeUploadBody streams the multipart body so that a large upload never has to
// be held in memory. It closes the multipart writer before returning.
func writeUploadBody(writer *multipart.Writer, files []UploadFile) error {
	for _, file := range files {
		name := strings.TrimSpace(file.Name)
		if name == "" {
			name = filepath.Base(file.Path)
		}

		part, err := writer.CreateFormFile("files", name)
		if err != nil {
			return fmt.Errorf("build upload body: %w", err)
		}
		if err := copyFileInto(part, file.Path); err != nil {
			return err
		}
	}
	return writer.Close()
}

// copyFileInto copies path into dst, always closing the file it opened.
func copyFileInto(dst io.Writer, path string) error {
	source, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	if _, err := io.Copy(dst, source); err != nil {
		source.Close()
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := source.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}
