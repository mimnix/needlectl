package cmd

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mimmo/needlectl/internal/api"
	"github.com/mimmo/needlectl/internal/output"
)

func TestTagEditModeMapsTheFlag(t *testing.T) {
	cases := map[string]string{
		renameModeSaveOverride: api.TagEditModeSaveOverride,
		renameModeWriteOnce:    api.TagEditModeWriteOnce,
	}
	for flag, want := range cases {
		got, err := tagEditMode(flag)
		if err != nil {
			t.Fatalf("tagEditMode(%q) returned error: %v", flag, err)
		}
		if got != want {
			t.Errorf("tagEditMode(%q) = %q, want %q", flag, got, want)
		}
	}

	if _, err := tagEditMode("nonsense"); err == nil {
		t.Error("tagEditMode(nonsense) returned nil error, want a failure")
	}
}

func TestPreviewBlocker(t *testing.T) {
	ready := &api.ManagementPreview{
		State: "ready", ReadyForConfirmation: true,
		OperationRowRevision: []byte("7"),
	}
	if err := previewBlocker(ready); err != nil {
		t.Errorf("ready preview reported a blocker: %v", err)
	}

	cases := map[string]*api.ManagementPreview{
		"missing preview": nil,
		"expired":         {Expired: true, ReadyForConfirmation: true, OperationRowRevision: []byte("7")},
		"stale":           {Stale: true, StaleReasons: []string{"album changed"}, ReadyForConfirmation: true, OperationRowRevision: []byte("7")},
		"no revision":     {ReadyForConfirmation: true},
		"not ready":       {State: "validating", OperationRowRevision: []byte("7")},
	}
	for name, preview := range cases {
		if err := previewBlocker(preview); err == nil {
			t.Errorf("%s: previewBlocker returned nil, want a blocker", name)
		}
	}
}

func TestPreviewBlockerMentionsStaleReasons(t *testing.T) {
	preview := &api.ManagementPreview{
		Stale: true, StaleReasons: []string{"album revision moved", "settings changed"},
	}
	err := previewBlocker(preview)
	if err == nil || !strings.Contains(err.Error(), "album revision moved") {
		t.Errorf("blocker = %v, want it to include the stale reasons", err)
	}
}

func TestIdentitySummary(t *testing.T) {
	review := "f27d3b91-3520-4d42-88e8-e9d933b8c4df"
	hold := &api.ManagementHold{Kind: "handing_back", HandingBack: true}

	got := identitySummary(api.LibraryAlbum{
		IdentityState:   "local_only",
		ManagementHold:  hold,
		PendingReviewID: &review,
	})
	want := "state local_only · management handing_back · review " + review
	if got != want {
		t.Errorf("identitySummary = %q, want %q", got, want)
	}

	if got := identitySummary(api.LibraryAlbum{}); got != "" {
		t.Errorf("identitySummary of an empty album = %q, want empty", got)
	}

	// A hold without a kind says nothing useful and must not render a blank part.
	if got := identitySummary(api.LibraryAlbum{ManagementHold: &api.ManagementHold{HandingBack: true}}); got != "" {
		t.Errorf("identitySummary of a kindless hold = %q, want empty", got)
	}
}

func TestRenameRefusedErrorExplainsTheIdentityGate(t *testing.T) {
	reason := "IDENTITY_NOT_ACCEPTED"
	review := "f27d3b91-3520-4d42-88e8-e9d933b8c4df"
	editor := &api.TagEditor{IdentityReason: &reason}
	detail := &api.LibraryAlbum{
		IdentityState:   "local_only",
		ManagementHold:  &api.ManagementHold{Kind: "handing_back"},
		PendingReviewID: &review,
	}

	err := renameRefusedError("album 楽園 — Fujifabric", editor, detail)
	if err == nil {
		t.Fatal("renameRefusedError returned nil")
	}
	for _, want := range []string{"album 楽園 — Fujifabric", "IDENTITY_NOT_ACCEPTED", "local_only", "handing_back", review, "resolve the album's identity"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestRenameRefusedErrorNamesATrackToo(t *testing.T) {
	err := renameRefusedError("track Disorder — 神崎エルザ", &api.TagEditor{}, nil)
	if err == nil {
		t.Fatal("renameRefusedError returned nil")
	}
	if !strings.Contains(err.Error(), "track Disorder — 神崎エルザ cannot be renamed") {
		t.Errorf("error %q should name the track", err)
	}
}

func TestRenameRefusedErrorWithoutDetailOrReason(t *testing.T) {
	err := renameRefusedError("album 楽園", &api.TagEditor{}, nil)
	if err == nil {
		t.Fatal("renameRefusedError returned nil")
	}
	if !strings.Contains(err.Error(), "IDENTITY_NOT_ACCEPTED") {
		t.Errorf("error %q should fall back to the default reason", err)
	}
	if strings.Contains(err.Error(), "·") {
		t.Errorf("error %q should not include an empty summary", err)
	}

	if err := renameRefusedError("album 楽園", nil, nil); err == nil {
		t.Fatal("renameRefusedError with a nil editor returned nil")
	}
}

func TestPrintTagRenamePreviewShowsTheChange(t *testing.T) {
	var buffer bytes.Buffer
	printer := output.New(&buffer, &buffer, output.Options{})

	printTagRenamePreview(printer, tagRename{
		Label: "album", Subject: "Elza", ID: "a1",
		Current: "Elza", New: "Elza (Deluxe)", Mode: renameModeSaveOverride,
	}, &api.PreviewRef{JobID: "job-1"})

	got := buffer.String()
	for _, want := range []string{"Elza", "Elza (Deluxe)", renameModeSaveOverride, "job-1"} {
		if !strings.Contains(got, want) {
			t.Errorf("preview %q does not contain %q", got, want)
		}
	}
}

func TestConfirmTagRenameRefusesWithoutATerminal(t *testing.T) {
	cmd := newRootCommand()
	cmd.SetIn(bytes.NewReader(nil))
	cmd.SetOut(&bytes.Buffer{})

	_, err := confirmTagRename(cmd, output.New(&bytes.Buffer{}, &bytes.Buffer{}, output.Options{}), tagRename{
		Current: "Elza", New: "Elza (Deluxe)", Warning: "writes the tags",
	})
	if err == nil || !strings.Contains(err.Error(), "refusing to rename") {
		t.Errorf("confirmTagRename error = %v, want the non-interactive refusal", err)
	}
}

func TestWaitForManagementPreviewPollsUntilReady(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/v3/library/management/previews/job-1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if requests <= 2 {
			_, _ = w.Write([]byte(`{"job_id":"job-1","state":"queued"}`))
			return
		}
		_, _ = w.Write([]byte(`{"job_id":"job-1","state":"ready","ready_for_confirmation":true,"operation_row_revision":7}`))
	}))
	defer server.Close()

	client, err := api.New(server.URL, false)
	if err != nil {
		t.Fatalf("api.New returned error: %v", err)
	}

	preview, err := waitForManagementPreview(context.Background(), client, "job-1",
		&api.ManagementPreview{JobID: "job-1", State: "queued"})
	if err != nil {
		t.Fatalf("waitForManagementPreview returned error: %v", err)
	}
	if preview.State != "ready" || !preview.ReadyForConfirmation {
		t.Errorf("preview = %+v, want the ready state", preview)
	}
	if requests != 3 {
		t.Errorf("polled the server %d times, want 3", requests)
	}
	if err := previewBlocker(preview); err != nil {
		t.Errorf("the polled preview reported a blocker: %v", err)
	}
}

func TestWaitForManagementPreviewReturnsAReadyPreviewImmediately(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("a ready preview must not be polled again")
	}))
	defer server.Close()

	client, err := api.New(server.URL, false)
	if err != nil {
		t.Fatalf("api.New returned error: %v", err)
	}

	ready := &api.ManagementPreview{
		State: "succeeded", ReadyForConfirmation: true, OperationRowRevision: []byte("7"),
	}
	preview, err := waitForManagementPreview(context.Background(), client, "job-1", ready)
	if err != nil {
		t.Fatalf("waitForManagementPreview returned error: %v", err)
	}
	if preview != ready {
		t.Errorf("preview = %+v, want the one passed in", preview)
	}
}

func TestWaitForManagementPreviewReturnsTerminalStatesForTheBlocker(t *testing.T) {
	client, err := api.New("http://localhost:1234", false)
	if err != nil {
		t.Fatalf("api.New returned error: %v", err)
	}

	preview, err := waitForManagementPreview(context.Background(), client, "job-1",
		&api.ManagementPreview{State: "validating"})
	if err != nil {
		t.Fatalf("waitForManagementPreview returned error: %v", err)
	}
	if preview.State != "validating" {
		t.Errorf("state = %q, want validating", preview.State)
	}
	if err := previewBlocker(preview); err == nil {
		t.Error("a validating preview should still be blocked")
	}
}
