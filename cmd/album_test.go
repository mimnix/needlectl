package cmd

import (
	"bytes"
	"testing"

	"github.com/mimmo/needlectl/internal/albums"
	"github.com/mimmo/needlectl/internal/api"
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
