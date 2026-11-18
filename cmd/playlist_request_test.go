package cmd

import "testing"

func TestPlaylistCommandRegistersRequest(t *testing.T) {
	playlist := newPlaylistCommand(&globalOptions{})

	for _, command := range playlist.Commands() {
		if command.Name() == "request" {
			return
		}
	}
	t.Error("playlist command does not register request")
}

func TestPlaylistRequestRequiresAPlaylist(t *testing.T) {
	root := newRootCommand()
	root.SetArgs([]string{"playlist", "request"})

	if err := root.Execute(); err == nil {
		t.Error("playlist request without a playlist returned no error")
	}
}
