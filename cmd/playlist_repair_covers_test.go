package cmd

import (
	"testing"

	"github.com/mimmo/needlectl/internal/covers"
)

func TestPlaylistRepairCoversValidate(t *testing.T) {
	if err := (playlistRepairCoversOptions{argument: "Anime"}).validate(); err != nil {
		t.Errorf("a playlist name was rejected: %v", err)
	}
	if err := (playlistRepairCoversOptions{argument: "pl-1", byID: true}).validate(); err != nil {
		t.Errorf("a playlist id was rejected: %v", err)
	}
	if err := (playlistRepairCoversOptions{allPlaylists: true}).validate(); err != nil {
		t.Errorf("--all-playlists alone was rejected: %v", err)
	}

	for name, input := range map[string]playlistRepairCoversOptions{
		"nothing":             {},
		"all with a name":     {argument: "Anime", allPlaylists: true},
		"all with an id":      {argument: "pl-1", byID: true, allPlaylists: true},
		"id without argument": {byID: true},
		"blank playlist name": {argument: "   "},
	} {
		if err := input.validate(); err == nil {
			t.Errorf("%s: validate returned no error", name)
		}
	}
}

func TestCoverRepairCountLabel(t *testing.T) {
	result := covers.Result{Total: 66, Repaired: 3, Unchanged: 63}

	if got, want := coverRepairCountLabel(result, false), "3 entries repaired · 63 unchanged"; got != want {
		t.Errorf("label = %q, want %q", got, want)
	}
	if got, want := coverRepairCountLabel(result, true), "3 entries to repair · 63 unchanged"; got != want {
		t.Errorf("dry-run label = %q, want %q", got, want)
	}
	if got := coverRepairCountLabel(covers.Result{}, false); got != "empty" {
		t.Errorf("empty label = %q, want empty", got)
	}
}

func TestPlaylistCommandRegistersRepairCovers(t *testing.T) {
	playlist := newPlaylistCommand(&globalOptions{})

	for _, command := range playlist.Commands() {
		if command.Name() == "repair-covers" {
			return
		}
	}
	t.Error("playlist command does not register repair-covers")
}
