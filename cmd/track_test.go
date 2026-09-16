package cmd

import (
	"bytes"
	"testing"

	"github.com/mimmo/needlectl/internal/api"
	"github.com/mimmo/needlectl/internal/tracks"
)

func TestTrackLabel(t *testing.T) {
	cases := []struct {
		track tracks.Track
		want  string
	}{
		{tracks.Track{Title: "Disorder", ArtistName: "神崎エルザ"}, "Disorder — 神崎エルザ"},
		{tracks.Track{Title: "Solo"}, "Solo"},
		{tracks.Track{ID: "track-1"}, "track-1"},
	}
	for _, tc := range cases {
		if got := trackLabel(tc.track); got != tc.want {
			t.Errorf("trackLabel(%+v) = %q, want %q", tc.track, got, tc.want)
		}
	}
}

func TestTrackCandidateLabelIncludesAlbum(t *testing.T) {
	candidate := tracks.Track{Title: "Intro", ArtistName: "LiSA", AlbumName: "Launcher"}
	if got, want := trackCandidateLabel(candidate), "Intro — LiSA (Launcher)"; got != want {
		t.Errorf("trackCandidateLabel = %q, want %q", got, want)
	}
}

func TestFormatDuration(t *testing.T) {
	cases := map[float64]string{
		0:    "",
		215:  "3:35",
		59.6: "1:00",
		3600: "60:00",
	}
	for seconds, want := range cases {
		if got := formatDuration(seconds); got != want {
			t.Errorf("formatDuration(%v) = %q, want %q", seconds, got, want)
		}
	}
}

func TestTrackDetails(t *testing.T) {
	if got := trackDetails(tracks.Track{Format: "FLAC", DurationSeconds: 215}); got != "FLAC · 3:35" {
		t.Errorf("trackDetails = %q", got)
	}
	if got := trackDetails(tracks.Track{}); got != "" {
		t.Errorf("trackDetails = %q, want empty", got)
	}
}

func TestToTrackCopiesFields(t *testing.T) {
	got := toTrack(api.LibraryTrack{
		ID: "track-1", Title: "Disorder", ArtistName: "神崎エルザ", AlbumTitle: "Elza",
		TrackNumber: 4, DiscNumber: 1, DurationSeconds: 215, Format: "FLAC",
	})
	want := tracks.Track{
		ID: "track-1", Title: "Disorder", ArtistName: "神崎エルザ", AlbumName: "Elza",
		TrackNumber: 4, DiscNumber: 1, DurationSeconds: 215, Format: "FLAC",
	}
	if got != want {
		t.Errorf("toTrack = %+v, want %+v", got, want)
	}
}

func TestTrackDeleteRequiresAnArgument(t *testing.T) {
	root := newRootCommand()
	root.SetArgs([]string{"track", "delete"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})

	if err := root.Execute(); err == nil {
		t.Fatal("track delete without an argument returned nil error, want a failure")
	}
}
