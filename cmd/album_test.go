package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mimmo/needlectl/internal/albums"
	"github.com/mimmo/needlectl/internal/api"
	"github.com/mimmo/needlectl/internal/output"
)

func TestAlbumLabel(t *testing.T) {
	cases := []struct {
		album albums.Album
		want  string
	}{
		{albums.Album{Title: "Elza", ArtistName: "神崎エルザ"}, "Elza — 神崎エルザ"},
		{albums.Album{Title: "Solo"}, "Solo"},
	}
	for _, tc := range cases {
		if got := albumLabel(tc.album); got != tc.want {
			t.Errorf("albumLabel(%+v) = %q, want %q", tc.album, got, tc.want)
		}
	}
}

func TestAlbumDetails(t *testing.T) {
	if got := albumDetails(albums.Album{Format: "FLAC", Year: 2018}); got != "FLAC · 2018" {
		t.Errorf("albumDetails = %q, want %q", got, "FLAC · 2018")
	}
	if got := albumDetails(albums.Album{Year: 2018}); got != "2018" {
		t.Errorf("albumDetails = %q, want %q", got, "2018")
	}
	if got := albumDetails(albums.Album{}); got != "" {
		t.Errorf("albumDetails = %q, want empty", got)
	}
}

func TestFileAndWantedActions(t *testing.T) {
	if fileAction(true) != "delete from disk" || fileAction(false) != "keep on disk" {
		t.Errorf("fileAction = %q / %q", fileAction(true), fileAction(false))
	}
	if wantedAction(true) != "stop future downloads" || wantedAction(false) != "keep wanted" {
		t.Errorf("wantedAction = %q / %q", wantedAction(true), wantedAction(false))
	}
}

func TestToAlbumCopiesNullableFields(t *testing.T) {
	format := "MP3"
	year := 2020
	got := toAlbum(api.LibraryAlbum{
		ID: "a", Title: "Elza", ArtistName: "神崎エルザ", TrackCount: 3, Format: &format, Year: &year,
	})
	if got.Format != "MP3" || got.Year != 2020 || got.TrackCount != 3 {
		t.Errorf("toAlbum = %+v", got)
	}

	empty := toAlbum(api.LibraryAlbum{ID: "b"})
	if empty.Format != "" || empty.Year != 0 {
		t.Errorf("toAlbum with nil fields = %+v", empty)
	}
}

func TestConfirmRenameRefusesWithoutATerminal(t *testing.T) {
	cmd := newRootCommand()
	cmd.SetIn(bytes.NewReader(nil))
	cmd.SetOut(&bytes.Buffer{})

	_, err := confirmRename(cmd, output.New(&bytes.Buffer{}, &bytes.Buffer{}, output.Options{}),
		"Elza", "Elza (Deluxe)", 2)
	if err == nil || !strings.Contains(err.Error(), "refusing to rename") {
		t.Errorf("confirmRename error = %v, want the non-interactive refusal", err)
	}
}

func TestIdentitySummary(t *testing.T) {
	review := "f27d3b91-3520-4d42-88e8-e9d933b8c4df"

	got := identitySummary(api.LibraryAlbum{
		AlbumIdentityState:   "local_only",
		IdentificationStatus: "needs_review",
		ReviewID:             &review,
	})
	want := "state local_only · identification needs_review · review " + review
	if got != want {
		t.Errorf("identitySummary = %q, want %q", got, want)
	}

	if got := identitySummary(api.LibraryAlbum{}); got != "" {
		t.Errorf("identitySummary of an empty album = %q, want empty", got)
	}
}

func TestRenameRefusedErrorExplainsTheIdentityGate(t *testing.T) {
	reason := "IDENTITY_NOT_ACCEPTED"
	review := "f27d3b91-3520-4d42-88e8-e9d933b8c4df"
	target := albums.Album{ID: "92b6", Title: "楽園", ArtistName: "Fujifabric"}
	editor := &api.TagEditor{IdentityReason: &reason}
	detail := &api.LibraryAlbum{
		AlbumIdentityState:   "local_only",
		IdentificationStatus: "needs_review",
		ReviewID:             &review,
	}

	err := renameRefusedError(target, editor, detail)
	if err == nil {
		t.Fatal("renameRefusedError returned nil")
	}
	for _, want := range []string{"楽園 — Fujifabric", "IDENTITY_NOT_ACCEPTED", "local_only", "needs_review", review, "resolve the album's identity"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestRenameRefusedErrorWithoutDetailOrReason(t *testing.T) {
	target := albums.Album{Title: "楽園"}

	err := renameRefusedError(target, &api.TagEditor{}, nil)
	if err == nil {
		t.Fatal("renameRefusedError returned nil")
	}
	if !strings.Contains(err.Error(), "IDENTITY_NOT_ACCEPTED") {
		t.Errorf("error %q should fall back to the default reason", err)
	}
	if strings.Contains(err.Error(), "·") {
		t.Errorf("error %q should not include an empty summary", err)
	}

	if err := renameRefusedError(target, nil, nil); err == nil {
		t.Fatal("renameRefusedError with a nil editor returned nil")
	}
}

func TestAlbumDeleteRequiresAnArgument(t *testing.T) {
	root := newRootCommand()
	root.SetArgs([]string{"album", "delete"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})

	if err := root.Execute(); err == nil {
		t.Fatal("album delete without an argument returned nil error, want a failure")
	}
}

func TestAlbumCommandRegistersRename(t *testing.T) {
	album := newAlbumCommand(&globalOptions{})

	for _, command := range album.Commands() {
		if command.Name() == "rename" {
			return
		}
	}
	t.Error("album command does not register rename")
}

func TestAlbumRenameRequiresTwoArguments(t *testing.T) {
	for _, args := range [][]string{
		{"album", "rename"},
		{"album", "rename", "Elza"},
	} {
		root := newRootCommand()
		root.SetArgs(args)
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})

		if err := root.Execute(); err == nil {
			t.Errorf("%v returned nil error, want a failure", args)
		}
	}
}

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

func TestPrintRenamePreviewShowsTheChange(t *testing.T) {
	var buffer bytes.Buffer
	printer := output.New(&buffer, &buffer, output.Options{})

	printRenamePreview(printer, albums.Album{ID: "a1", Title: "Elza"}, "Elza", "Elza (Deluxe)", renameModeSaveOverride,
		&api.PreviewRef{JobID: "job-1"})

	got := buffer.String()
	for _, want := range []string{"Elza", "Elza (Deluxe)", renameModeSaveOverride, "job-1"} {
		if !strings.Contains(got, want) {
			t.Errorf("preview %q does not contain %q", got, want)
		}
	}
}
