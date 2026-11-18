package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/mimmo/needlectl/internal/api"
	"github.com/mimmo/needlectl/internal/mbref"
	"github.com/mimmo/needlectl/internal/output"
	"github.com/spf13/cobra"
)

func newHeldCommand(opts *globalOptions) *cobra.Command {
	held := &cobra.Command{
		Use:   "held",
		Short: "Inspect and resolve entries waiting for a decision",
		RunE:  groupRunE,
		Long: `Work with the entries DroppedNeedle kept out of the library because it could
not accept an album for them.

These are the "unmatched" files: a download or upload whose tags, AcoustID
result or release fit did not convince the server. Nothing is lost — the files
stay on disk, and every entry carries the reason it was held.

"needlectl held list" shows them; "needlectl held match" imports one as the
MusicBrainz album you name.`,
	}
	held.AddCommand(newHeldListCommand(opts))
	held.AddCommand(newHeldMatchCommand(opts))
	return held
}

type heldListOptions struct {
	all bool
}

func newHeldListCommand(opts *globalOptions) *cobra.Command {
	input := heldListOptions{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the entries waiting for a decision",
		Long: `List the downloads and uploads DroppedNeedle held back, newest first.

Each entry is shown with the album it was thought to be, the reason it was held
and its files. Entries are identified by an entry id, which "held match" takes.

Your own entries are listed. Admins can pass --all to see every user's.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHeldList(cmd, opts, input)
		},
	}

	cmd.Flags().BoolVar(&input.all, "all", false, "list every user's entries instead of only yours (admins)")

	return cmd
}

func runHeldList(cmd *cobra.Command, opts *globalOptions, input heldListOptions) error {
	printer := opts.printer()

	cfg, err := opts.loadConfig(printer)
	if err != nil {
		return err
	}

	printer.Step("Authenticating with %s", strings.TrimRight(cfg.Server, "/"))
	client, err := api.New(cfg.Server, cfg.InsecureSkipVerify)
	if err != nil {
		return err
	}
	user, err := client.Login(cmd.Context(), cfg.Username, cfg.Password)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}
	printer.Success("Signed in as %s (%s)", user.DisplayName, user.Role)

	printer.Step("Loading held entries")
	entries, err := client.ListHeldEntries(cmd.Context(), input.all)
	if err != nil {
		return fmt.Errorf("list held entries: %w", err)
	}

	if len(entries) == 0 {
		printer.Info("no held entries found")
		return nil
	}

	for _, entry := range entries {
		printHeldEntry(printer, entry)
	}

	printer.Raw("")
	printer.Success("%s waiting for a decision", output.CountLabel(len(entries), "entry"))
	printer.Info("link one with: needlectl held match <entry-id> <musicbrainz release or release-group URL/MBID>")
	return nil
}

type heldMatchOptions struct {
	entry     string
	reference string
	yes       bool
	dryRun    bool
}

func newHeldMatchCommand(opts *globalOptions) *cobra.Command {
	input := heldMatchOptions{}

	cmd := &cobra.Command{
		Use:   "match <entry-id> <reference>",
		Short: "Import a held entry as a specific MusicBrainz album",
		Long: `Import the files of one held entry into the library as the album you name.

This is the manual decision the DroppedNeedle web UI offers on a held entry: the
files are paired with the album's tracks and go into the library. Unlike the
automatic check, the match is not refused because the files' tags disagree with
the release.

DroppedNeedle links held files by release group, so the reference accepts a
MusicBrainz release URL, a release-group URL or a bare MBID; a release is
resolved to its group with a single lookup on musicbrainz.org.

Matching imports the files; it downloads nothing. Use --dry-run to resolve the
reference and show what would be imported.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			input.entry = args[0]
			input.reference = args[1]
			return runHeldMatch(cmd, opts, input)
		},
	}

	cmd.Flags().BoolVarP(&input.yes, "yes", "y", false, "do not ask for confirmation")
	cmd.Flags().BoolVar(&input.dryRun, "dry-run", false, "resolve the reference and show what would be imported without importing it")

	return cmd
}

func runHeldMatch(cmd *cobra.Command, opts *globalOptions, input heldMatchOptions) error {
	entryID, err := parseHeldEntryID(input.entry)
	if err != nil {
		return err
	}

	printer := opts.printer()

	cfg, err := opts.loadConfig(printer)
	if err != nil {
		return err
	}

	printer.Step("Authenticating with %s", strings.TrimRight(cfg.Server, "/"))
	client, err := api.New(cfg.Server, cfg.InsecureSkipVerify)
	if err != nil {
		return err
	}
	user, err := client.Login(cmd.Context(), cfg.Username, cfg.Password)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}
	printer.Success("Signed in as %s (%s)", user.DisplayName, user.Role)

	printer.Step("Loading held entry %d", entryID)
	entries, err := client.ListHeldEntries(cmd.Context(), false)
	if err != nil {
		return fmt.Errorf("list held entries: %w", err)
	}
	entry, ok := findHeldEntry(entries, entryID)
	if !ok {
		return fmt.Errorf("no held entry %d; run 'needlectl held list' to see the entries waiting for a decision", entryID)
	}
	printHeldEntry(printer, entry)

	reference, err := mbref.Parse(input.reference)
	if err != nil {
		return err
	}
	var releaseGroup string
	if reference.Kind == mbref.KindReleaseGroup {
		printer.Step("Using release group %s", reference.MBID)
		releaseGroup = reference.MBID
	} else {
		printer.Step("Resolving %s in MusicBrainz", input.reference)
		releaseGroup, err = mbref.NewResolver().ResolveReleaseGroup(cmd.Context(), reference)
		if err != nil {
			return fmt.Errorf("resolve MusicBrainz reference: %w", err)
		}
		printer.Success("Resolved to release group %s", releaseGroup)
	}

	if entry.ReleaseGroup() != "" && entry.ReleaseGroup() == releaseGroup {
		printer.Warn("this is the release group DroppedNeedle already guessed; the match imports the files anyway")
	}

	printer.Raw("")
	printer.Field("release", releaseGroup)
	printer.Field("files", strconv.Itoa(len(entry.Files)))

	if input.dryRun {
		printer.Raw("")
		printer.Success("Dry run complete — nothing was imported")
		return nil
	}

	if !input.yes {
		confirmed, err := confirmHeldMatch(cmd, printer, entry, releaseGroup)
		if err != nil {
			return err
		}
		if !confirmed {
			printer.Warn("aborted — nothing was imported")
			return nil
		}
	}

	printer.Step("Importing held entry %d as release group %s", entryID, releaseGroup)
	result, err := client.MatchHeldEntry(cmd.Context(), entryID, releaseGroup)
	if err != nil {
		return fmt.Errorf("match held entry: %w", err)
	}

	printer.Raw("")
	printer.Success("Held entry %d resolved", entryID)
	printer.Field("imported", strconv.Itoa(result.Imported))
	if result.AlreadyOwned > 0 {
		printer.Field("already owned", strconv.Itoa(result.AlreadyOwned))
	}
	if result.StillHeld > 0 {
		printer.Field("still held", strconv.Itoa(result.StillHeld))
	}
	if result.Discarded > 0 {
		printer.Field("discarded", strconv.Itoa(result.Discarded))
	}
	if result.Message != nil && strings.TrimSpace(*result.Message) != "" {
		printer.Info("%s", strings.TrimSpace(*result.Message))
	}
	if result.StillHeld > 0 {
		printer.Warn("%s could not be paired with the album and stayed held; decide them in the DroppedNeedle web UI",
			output.CountLabel(result.StillHeld, "file"))
	}
	return nil
}

// parseHeldEntryID reads the entry id argument, rejecting anything that is not a
// positive integer.
func parseHeldEntryID(raw string) (int64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, errors.New("the held entry id is empty")
	}
	id, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%q is not a held entry id; run 'needlectl held list' to see the ids", raw)
	}
	return id, nil
}

// findHeldEntry returns the entry with the given id.
func findHeldEntry(entries []api.HeldEntry, id int64) (api.HeldEntry, bool) {
	for _, entry := range entries {
		if entry.ID == id {
			return entry, true
		}
	}
	return api.HeldEntry{}, false
}

func printHeldEntry(printer *output.Printer, entry api.HeldEntry) {
	printer.Raw("")
	printer.Field("entry", strconv.FormatInt(entry.ID, 10))
	if label := heldEntryLabel(entry); label != "" {
		printer.Field("album", label)
	}
	printer.Field("state", heldEntryState(entry))
	printer.Field("files", strconv.Itoa(len(entry.Files)))
	if group := entry.ReleaseGroup(); group != "" {
		printer.Field("guess", group)
	}
	for _, file := range entry.Files {
		printer.Info("%s", heldFileLabel(file))
		if details := heldFileDetails(file); details != "" {
			printer.Info("%s", details)
		}
		if text := strings.TrimSpace(file.ReasonText); text != "" {
			printer.Info("%s", text)
		}
	}
}

// heldEntryLabel renders "Album — Artist", dropping the parts that are missing.
func heldEntryLabel(entry api.HeldEntry) string {
	title := strings.TrimSpace(entry.Title())
	artist := strings.TrimSpace(entry.Artist())
	switch {
	case title == "" && artist == "":
		return ""
	case artist == "":
		return title
	case title == "":
		return artist
	default:
		return fmt.Sprintf("%s — %s", title, artist)
	}
}

// heldEntryState renders the entry state with the reason it was held.
func heldEntryState(entry api.HeldEntry) string {
	state := strings.TrimSpace(entry.State)
	if state == "" {
		state = "held"
	}
	if reason := strings.TrimSpace(entry.ReasonCode()); reason != "" {
		return state + " · " + reason
	}
	return state
}

// heldFileLabel renders one held file's name, format, length and track number.
func heldFileLabel(file api.HeldFile) string {
	parts := []string{file.OriginalName}
	if format := strings.TrimSpace(heldStringValue(file.Format)); format != "" {
		parts = append(parts, format)
	}
	if file.DurationSeconds != nil {
		if duration := formatDuration(*file.DurationSeconds); duration != "" {
			parts = append(parts, duration)
		}
	}
	if file.TrackNumber != nil && *file.TrackNumber > 0 {
		parts = append(parts, "track "+strconv.Itoa(*file.TrackNumber))
	}
	return strings.Join(parts, " · ")
}

// heldFileDetails renders where a held file came from and which release it was
// checked against.
func heldFileDetails(file api.HeldFile) string {
	parts := make([]string, 0, 2)
	if source := strings.TrimSpace(file.Source); source != "" {
		parts = append(parts, "from "+source)
	}
	if release := strings.TrimSpace(heldStringValue(file.ReleaseMBID)); release != "" {
		parts = append(parts, "release "+release)
	}
	return strings.Join(parts, " · ")
}

// heldStringValue returns the string behind an optional field, or "" when it is
// missing.
func heldStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func confirmHeldMatch(cmd *cobra.Command, printer *output.Printer, entry api.HeldEntry, releaseGroup string) (bool, error) {
	stdinFile, isFile := cmd.InOrStdin().(*os.File)
	if !isFile || !output.IsTerminal(stdinFile) {
		return false, errors.New("refusing to import without confirmation; re-run with --yes")
	}

	printer.Raw("")
	printer.Warn("this imports the entry's %s into the library as release group %s",
		output.CountLabel(len(entry.Files), "file"), releaseGroup)

	fmt.Fprintf(cmd.OutOrStdout(), "Import held entry %d? [y/N]: ", entry.ID)
	reader := bufio.NewReader(cmd.InOrStdin())
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("read confirmation: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}
