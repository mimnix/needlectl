package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mimmo/needlectl/internal/importer"
	"github.com/mimmo/needlectl/internal/output"
)

func TestDefaultPlaylistNameStripsExtension(t *testing.T) {
	cases := map[string]string{
		"/home/user/My Spotify Library.csv": "My Spotify Library",
		"playlist.CSV":                      "playlist",
		"no-extension":                      "no-extension",
	}
	for input, want := range cases {
		if got := defaultPlaylistName(input); got != want {
			t.Errorf("defaultPlaylistName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRootCommandRegistersSubcommands(t *testing.T) {
	root := newRootCommand()

	wanted := map[string]bool{"login": false, "import": false, "album": false, "track": false, "playlist": false}
	for _, command := range root.Commands() {
		if _, ok := wanted[command.Name()]; ok {
			wanted[command.Name()] = true
		}
	}
	for name, found := range wanted {
		if !found {
			t.Errorf("root command does not register %q", name)
		}
	}
}

func TestImportCommandRegistersSubcommands(t *testing.T) {
	importCmd := newImportCommand(&globalOptions{})

	wanted := map[string]bool{"playlist": false, "tracks": false, "jobs": false}
	for _, command := range importCmd.Commands() {
		if _, ok := wanted[command.Name()]; ok {
			wanted[command.Name()] = true
		}
	}
	for name, found := range wanted {
		if !found {
			t.Errorf("import command does not register %q", name)
		}
	}
}

func TestImportCommandWithoutArgumentsShowsHelp(t *testing.T) {
	var out bytes.Buffer
	root := newRootCommand()
	root.SetArgs([]string{"import"})
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("import without arguments returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{"tracks", "jobs"} {
		if !strings.Contains(got, want) {
			t.Errorf("help output does not mention %q:\n%s", want, got)
		}
	}
}

func TestImportCommandRejectsMoreThanOnePlaylist(t *testing.T) {
	root := newRootCommand()
	root.SetArgs([]string{"import", "one.csv", "two.csv"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})

	if err := root.Execute(); err == nil {
		t.Fatal("import with two arguments returned nil error, want a failure")
	}
}

func TestImportPlaylistSubcommandRequiresAnArgument(t *testing.T) {
	root := newRootCommand()
	root.SetArgs([]string{"import", "playlist"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})

	if err := root.Execute(); err == nil {
		t.Fatal("import playlist without an argument returned nil error, want a failure")
	}
}

func TestPrintSummaryReportsCounts(t *testing.T) {
	var buffer bytes.Buffer
	printer := output.New(&buffer, &buffer, output.Options{})

	printSummary(printer, importer.Result{
		PlaylistName:  "Anime",
		PlaylistID:    "pl-1",
		LibrarySize:   3,
		AddedLinked:   2,
		AddedUnlinked: 1,
		Unchanged:     5,
		Playable:      2,
		StillUnlinked: 1,
	}, false)

	got := buffer.String()
	for _, want := range []string{"Anime", "pl-1", "3 (2 linked)", "5"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary %q does not contain %q", got, want)
		}
	}
}
