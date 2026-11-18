package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mimnix/needlectl/internal/output"
	"github.com/mimnix/needlectl/internal/relink"
)

func TestPlaylistRelinkRequiresAnArgument(t *testing.T) {
	root := newRootCommand()
	root.SetArgs([]string{"playlist", "relink"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})

	if err := root.Execute(); err == nil {
		t.Fatal("playlist relink without an argument returned nil error, want a failure")
	}
}

func TestPlaylistCommandRegistersRelink(t *testing.T) {
	playlist := newPlaylistCommand(&globalOptions{})

	for _, command := range playlist.Commands() {
		if command.Name() == "relink" {
			return
		}
	}
	t.Error("playlist command does not register relink")
}

func TestPrintRelinkSummaryReportsCounts(t *testing.T) {
	var buffer bytes.Buffer
	printer := output.New(&buffer, &buffer, output.Options{})

	printRelinkSummary(printer, relink.Result{
		PlaylistName:  "Anime",
		PlaylistID:    "pl-1",
		LibrarySize:   10,
		Total:         8,
		Playable:      5,
		Relinked:      2,
		Unmatched:     1,
		PlayableAfter: 7,
		StillUnlinked: 1,
	}, false)

	got := buffer.String()
	for _, want := range []string{"Relink complete", "Anime", "pl-1", "10 tracks", "playable now"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary %q does not contain %q", got, want)
		}
	}
}

func TestPrintRelinkSummaryForDryRunOmitsTheAfterState(t *testing.T) {
	var buffer bytes.Buffer
	printer := output.New(&buffer, &buffer, output.Options{})

	printRelinkSummary(printer, relink.Result{PlaylistName: "Anime", Relinked: 1}, true)

	got := buffer.String()
	if !strings.Contains(got, "Dry run complete") {
		t.Errorf("summary %q does not mention the dry run", got)
	}
	if strings.Contains(got, "playable now") {
		t.Errorf("dry run summary %q should not report the after state", got)
	}
}
