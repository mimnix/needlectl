package cmd

import (
	"bytes"
	"testing"
)

func TestGroupsRejectUnknownOperations(t *testing.T) {
	cases := [][]string{
		{"import", "playlist", "list.csv"}, // CSV import moved to playlist import
		{"import", "tracks", "song.mp3"},   // renamed to import upload
		{"import", "jobs"},                 // renamed to import list
		{"import", "x.csv"},                // bare import alias removed
		{"playlist", "covers"},             // renamed to repair-covers
		{"playlist", "download"},           // renamed to request
		{"album", "nonsense"},
		{"track", "nonsense"},
		{"held", "nonsense"},
		{"nonsense"},
	}

	for _, args := range cases {
		root := newRootCommand()
		root.SetArgs(args)
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})

		if err := root.Execute(); err == nil {
			t.Errorf("%v returned nil error, want an unknown-command failure", args)
		}
	}
}

func TestGroupsShowHelpWithoutAnOperation(t *testing.T) {
	for _, args := range [][]string{{}, {"import"}, {"playlist"}, {"album"}, {"track"}, {"held"}} {
		root := newRootCommand()
		root.SetArgs(args)
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})

		if err := root.Execute(); err != nil {
			t.Errorf("%v returned error: %v", args, err)
		}
	}
}
