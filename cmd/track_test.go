package cmd

import (
	"bytes"
	"strings"
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

func TestTrackCandidateLabelShowsWhatTellsDuplicatesApart(t *testing.T) {
	cases := []struct {
		name  string
		track tracks.Track
		want  string
	}{
		{
			name:  "album only",
			track: tracks.Track{Title: "Intro", ArtistName: "LiSA", AlbumName: "Launcher"},
			want:  "Intro — LiSA (Launcher)",
		},
		{
			name: "same title and album, distinguished by disc, track, format and duration",
			track: tracks.Track{
				Title: "Intro", ArtistName: "LiSA", AlbumName: "Launcher",
				DiscNumber: 2, TrackNumber: 3, Format: "FLAC", DurationSeconds: 215,
			},
			want: "Intro — LiSA (Launcher) · disc 2 track 3 · FLAC · 3:35",
		},
		{
			name: "single disc omits the disc number",
			track: tracks.Track{
				Title: "Intro", ArtistName: "LiSA", AlbumName: "Launcher",
				DiscNumber: 1, TrackNumber: 3, Format: "MP3",
			},
			want: "Intro — LiSA (Launcher) · track 3 · MP3",
		},
	}
	for _, tc := range cases {
		if got := trackCandidateLabel(tc.track); got != tc.want {
			t.Errorf("%s: trackCandidateLabel = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestTrackPosition(t *testing.T) {
	cases := []struct {
		track tracks.Track
		want  string
	}{
		{tracks.Track{DiscNumber: 1, TrackNumber: 3}, "track 3"},
		{tracks.Track{DiscNumber: 2, TrackNumber: 3}, "disc 2 track 3"},
		{tracks.Track{DiscNumber: 2}, "disc 2"},
		{tracks.Track{}, ""},
	}
	for _, tc := range cases {
		if got := trackPosition(tc.track); got != tc.want {
			t.Errorf("trackPosition(%+v) = %q, want %q", tc.track, got, tc.want)
		}
	}
}

func TestAmbiguityHintPointsAtTheUsefulFlag(t *testing.T) {
	sameAlbum := []tracks.Track{
		{Title: "Intro", ArtistName: "LiSA", AlbumName: "Launcher"},
		{Title: "Intro", ArtistName: "LiSA", AlbumName: "Launcher"},
	}
	if hint := ambiguityHint(sameAlbum); !strings.Contains(hint, "--track") {
		t.Errorf("hint for same-album matches = %q, want it to suggest --track", hint)
	}

	otherAlbums := []tracks.Track{
		{Title: "Intro", ArtistName: "LiSA", AlbumName: "Launcher"},
		{Title: "Intro", ArtistName: "LiSA", AlbumName: "Other"},
	}
	if hint := ambiguityHint(otherAlbums); !strings.Contains(hint, "--artist/--album") {
		t.Errorf("hint for different albums = %q, want it to suggest --artist/--album", hint)
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
		RowRevision: 23,
	})
	want := tracks.Track{
		ID: "track-1", Title: "Disorder", ArtistName: "神崎エルザ", AlbumName: "Elza",
		TrackNumber: 4, DiscNumber: 1, DurationSeconds: 215, Format: "FLAC",
		Revision: 23,
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

func TestTrackDeleteRejectsFlagsThatWouldBeIgnored(t *testing.T) {
	cases := [][]string{
		{"track", "delete", "abc", "--id", "--track", "3"},
		{"track", "delete", "abc", "--id", "--disc", "2"},
		{"track", "delete", "abc", "--id", "--artist", "LiSA"},
		{"track", "delete", "abc", "--id", "--album", "Launcher"},
	}
	for _, args := range cases {
		root := newRootCommand()
		root.SetArgs(args)
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})

		err := root.Execute()
		if err == nil {
			t.Errorf("%v returned nil error, want a failure", args)
			continue
		}
		if !strings.Contains(err.Error(), "cannot be combined") {
			t.Errorf("%v error = %q, want the conflicting-flags message", args, err)
		}
	}
}

func TestValidateTrackFilter(t *testing.T) {
	if err := validateTrackFilter(false, "LiSA", "Launcher", 3, 1); err != nil {
		t.Errorf("narrowing without --id returned error: %v", err)
	}
	if err := validateTrackFilter(true, "", "", 0, 0); err != nil {
		t.Errorf("--id alone returned error: %v", err)
	}

	for _, tc := range []struct {
		artist string
		album  string
		track  int
		disc   int
	}{
		{artist: "LiSA"},
		{album: "Launcher"},
		{track: 3},
		{disc: 2},
	} {
		if err := validateTrackFilter(true, tc.artist, tc.album, tc.track, tc.disc); err == nil {
			t.Errorf("validateTrackFilter(--id, %+v) returned nil, want a failure", tc)
		}
	}
}

func TestTrackCommandRegistersRename(t *testing.T) {
	track := newTrackCommand(&globalOptions{})

	for _, command := range track.Commands() {
		if command.Name() == "rename" {
			return
		}
	}
	t.Error("track command does not register rename")
}

func TestTrackRenameRequiresTwoArguments(t *testing.T) {
	for _, args := range [][]string{
		{"track", "rename"},
		{"track", "rename", "Disorder"},
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

func TestTrackRenameRejectsIDWithNarrowingFlags(t *testing.T) {
	root := newRootCommand()
	root.SetArgs([]string{"track", "rename", "abc", "New Title", "--id", "--track", "3"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Errorf("error = %v, want the conflicting-flags message", err)
	}
}
