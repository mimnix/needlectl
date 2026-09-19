package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mimmo/needlectl/internal/api"
)

func TestCollectUploadFilesTakesFileArguments(t *testing.T) {
	dir := t.TempDir()
	path := writeUploadFixture(t, dir, "song.mp3")

	files, err := collectUploadFiles([]string{path})
	if err != nil {
		t.Fatalf("collectUploadFiles returned error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(files))
	}
	if files[0].Name != "song.mp3" || files[0].Path != path {
		t.Errorf("file = %+v, want song.mp3 at %s", files[0], path)
	}
}

func TestCollectUploadFilesWalksDirectoriesInOrder(t *testing.T) {
	dir := t.TempDir()
	writeUploadFixture(t, dir, "b.mp3")
	writeUploadFixture(t, dir, "a.mp3")
	nested := filepath.Join(dir, "cd")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeUploadFixture(t, nested, "c.mp3")

	files, err := collectUploadFiles([]string{dir})
	if err != nil {
		t.Fatalf("collectUploadFiles returned error: %v", err)
	}

	names := make([]string, 0, len(files))
	for _, file := range files {
		names = append(names, file.Name)
	}
	want := []string{"a.mp3", "b.mp3", "c.mp3"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("names = %v, want %v", names, want)
	}
}

func TestCollectUploadFilesSkipsHiddenEntries(t *testing.T) {
	dir := t.TempDir()
	writeUploadFixture(t, dir, "song.mp3")
	writeUploadFixture(t, dir, ".hidden.mp3")

	hiddenDir := filepath.Join(dir, ".cache")
	if err := os.MkdirAll(hiddenDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeUploadFixture(t, hiddenDir, "junk.mp3")

	files, err := collectUploadFiles([]string{dir})
	if err != nil {
		t.Fatalf("collectUploadFiles returned error: %v", err)
	}
	if len(files) != 1 || files[0].Name != "song.mp3" {
		t.Errorf("files = %+v, want only song.mp3", files)
	}
}

func TestCollectUploadFilesRejectsMissingPaths(t *testing.T) {
	if _, err := collectUploadFiles([]string{filepath.Join(t.TempDir(), "absent")}); err == nil {
		t.Fatal("collectUploadFiles returned nil error, want a failure")
	}
}

func TestCollectUploadFilesRejectsEmptySelections(t *testing.T) {
	if _, err := collectUploadFiles([]string{t.TempDir()}); err == nil {
		t.Fatal("collectUploadFiles returned nil error, want a failure")
	}
}

func TestImportTracksCommandRequiresPaths(t *testing.T) {
	root := newRootCommand()
	root.SetArgs([]string{"import", "tracks"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})

	if err := root.Execute(); err == nil {
		t.Fatal("import tracks without a path returned nil error, want a failure")
	}
}

func TestImportTracksDryRunWritesNothing(t *testing.T) {
	printer, out := testPrinter()

	err := printImportTracksDryRun(printer, []api.UploadFile{{Name: "song.mp3", Path: "/music/song.mp3"}}, "e49c0013")
	if err != nil {
		t.Fatalf("printImportTracksDryRun returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{"Dry run", "/music/song.mp3", "e49c0013"} {
		if !strings.Contains(got, want) {
			t.Errorf("dry run output %q does not contain %q", got, want)
		}
	}
}

// writeUploadFixture creates a file inside dir and returns its path.
func writeUploadFixture(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("audio"), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}
