package cmd

import (
	"strings"
	"testing"

	"github.com/mimmo/needlectl/internal/api"
)

func TestParseHeldEntryID(t *testing.T) {
	if id, err := parseHeldEntryID(" 12 "); err != nil || id != 12 {
		t.Errorf("parseHeldEntryID(12) = %d, %v; want 12", id, err)
	}
	for _, raw := range []string{"", "  ", "0", "-1", "abc", "1.5"} {
		if _, err := parseHeldEntryID(raw); err == nil {
			t.Errorf("parseHeldEntryID(%q) returned no error", raw)
		}
	}
}

func TestFindHeldEntry(t *testing.T) {
	entries := []api.HeldEntry{{ID: 1}, {ID: 2}}

	if entry, ok := findHeldEntry(entries, 2); !ok || entry.ID != 2 {
		t.Errorf("findHeldEntry(2) = %+v, %v; want entry 2", entry, ok)
	}
	if _, ok := findHeldEntry(entries, 3); ok {
		t.Error("findHeldEntry(3) reported an entry")
	}
}

func TestHeldEntryLabelDropsMissingParts(t *testing.T) {
	title := "Strike"
	artist := "Far Out"

	cases := []struct {
		name  string
		entry api.HeldEntry
		want  string
	}{
		{"both", api.HeldEntry{AlbumTitle: &title, ArtistName: &artist}, "Strike — Far Out"},
		{"title only", api.HeldEntry{AlbumTitle: &title}, "Strike"},
		{"artist only", api.HeldEntry{ArtistName: &artist}, "Far Out"},
		{"neither", api.HeldEntry{}, ""},
	}
	for _, tc := range cases {
		if got := heldEntryLabel(tc.entry); got != tc.want {
			t.Errorf("%s: heldEntryLabel = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestHeldEntryStateShowsTheReason(t *testing.T) {
	entry := api.HeldEntry{
		State: "held",
		Files: []api.HeldFile{{Reason: "weak_match"}},
	}
	if got := heldEntryState(entry); got != "held · weak_match" {
		t.Errorf("heldEntryState = %q, want the reason appended", got)
	}

	if got := heldEntryState(api.HeldEntry{}); got != "held" {
		t.Errorf("heldEntryState of an empty entry = %q, want held", got)
	}
}

func TestHeldFileLabelDescribesAFile(t *testing.T) {
	format := "mp3"
	duration := 191.269
	track := 110

	file := api.HeldFile{
		OriginalName:    "110. Far Out - Strike.mp3",
		Format:          &format,
		DurationSeconds: &duration,
		TrackNumber:     &track,
	}
	got := heldFileLabel(file)
	for _, want := range []string{"110. Far Out - Strike.mp3", "mp3", "3:11", "track 110"} {
		if !strings.Contains(got, want) {
			t.Errorf("heldFileLabel = %q, want it to contain %q", got, want)
		}
	}

	bare := api.HeldFile{OriginalName: "track.flac"}
	if got := heldFileLabel(bare); got != "track.flac" {
		t.Errorf("heldFileLabel of a bare file = %q, want the name only", got)
	}
}

func TestHeldFileDetailsShowsSourceAndRelease(t *testing.T) {
	release := "ae891a44-138f-4076-b4af-fe29af2f1a5a"
	file := api.HeldFile{Source: "soulseek", ReleaseMBID: &release}

	if got := heldFileDetails(file); got != "from soulseek · release "+release {
		t.Errorf("heldFileDetails = %q", got)
	}
	if got := heldFileDetails(api.HeldFile{}); got != "" {
		t.Errorf("heldFileDetails of an empty file = %q, want empty", got)
	}
}

func TestHeldCommandRegistersListAndMatch(t *testing.T) {
	command := newHeldCommand(&globalOptions{})

	names := map[string]bool{}
	for _, sub := range command.Commands() {
		names[sub.Name()] = true
	}
	for _, want := range []string{"list", "match"} {
		if !names[want] {
			t.Errorf("held command is missing %q", want)
		}
	}
}
