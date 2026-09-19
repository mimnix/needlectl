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
	"github.com/mimmo/needlectl/internal/matcher"
	"github.com/mimmo/needlectl/internal/output"
	"github.com/mimmo/needlectl/internal/tracks"
	"github.com/spf13/cobra"
)

func newTrackCommand(opts *globalOptions) *cobra.Command {
	track := &cobra.Command{
		Use:   "track",
		Short: "Inspect and manage tracks in the DroppedNeedle library",
	}
	track.AddCommand(newTrackDeleteCommand(opts))
	track.AddCommand(newTrackRenameCommand(opts))
	return track
}

type trackDeleteOptions struct {
	argument string
	byID     bool
	artist   string
	album    string
	track    int
	disc     int
	yes      bool
	dryRun   bool
}

// validate rejects flag combinations that would silently ignore part of the
// request on a destructive command.
func (i trackDeleteOptions) validate() error {
	return validateTrackFilter(i.byID, i.artist, i.album, i.track, i.disc)
}

// validateTrackFilter rejects the narrowing flags when --id is used, because
// they would be silently ignored.
func validateTrackFilter(byID bool, artist, album string, track, disc int) error {
	if !byID {
		return nil
	}
	if strings.TrimSpace(artist) != "" || strings.TrimSpace(album) != "" || track > 0 || disc > 0 {
		return errors.New("--id cannot be combined with --artist, --album, --track or --disc")
	}
	return nil
}

func newTrackDeleteCommand(opts *globalOptions) *cobra.Command {
	input := trackDeleteOptions{}

	cmd := &cobra.Command{
		Use:   "delete <track>",
		Short: "Delete a single track from the library",
		Long: `Delete a single track from the DroppedNeedle library.

WARNING: the DroppedNeedle API has no catalog-only option for a single track,
so the audio file is always deleted from disk. Use "album delete --keep-files"
if you only want to remove catalog entries.

The track is looked up by name and must match exactly one library track. Use
--artist and/or --album to narrow the search, --track (and --disc when the same
track number appears on several discs) to pick between tracks that share a name
on one album, or --id to pass an exact track id.

When several tracks match, they are listed with their album, disc/track number,
format and duration so they can be told apart; nothing is deleted.

Deletion is confirmed interactively unless --yes is given. In a non-interactive
session --yes is required.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input.argument = args[0]
			return runTrackDelete(cmd, opts, input)
		},
	}

	cmd.Flags().BoolVar(&input.byID, "id", false, "treat the argument as an exact track id and skip the library lookup")
	cmd.Flags().StringVar(&input.artist, "artist", "", "restrict the track lookup to this artist")
	cmd.Flags().StringVar(&input.album, "album", "", "restrict the track lookup to this album")
	cmd.Flags().IntVar(&input.track, "track", 0, "restrict the track lookup to this track number")
	cmd.Flags().IntVar(&input.disc, "disc", 0, "restrict the track lookup to this disc number")
	cmd.Flags().BoolVarP(&input.yes, "yes", "y", false, "do not ask for confirmation")
	cmd.Flags().BoolVar(&input.dryRun, "dry-run", false, "show what would be deleted without deleting anything")

	return cmd
}

func runTrackDelete(cmd *cobra.Command, opts *globalOptions, input trackDeleteOptions) error {
	if err := input.validate(); err != nil {
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

	target, err := resolveTrackTarget(cmd, printer, client, input.argument, input.byID, tracks.Criteria{
		Artist: input.artist,
		Album:  input.album,
		Track:  input.track,
		Disc:   input.disc,
	})
	if err != nil {
		return err
	}
	printTrackTarget(printer, target)

	if input.dryRun {
		printer.Raw("")
		printer.Success("Dry run complete — nothing was deleted")
		printer.Field("file", "delete from disk")
		return nil
	}

	if !input.yes {
		confirmed, err := confirmTrackDeletion(cmd, printer, target)
		if err != nil {
			return err
		}
		if !confirmed {
			printer.Warn("aborted — nothing was deleted")
			return nil
		}
	}

	printer.Step("Deleting track %s", trackLabel(target))
	removal, err := client.DeleteTrack(cmd.Context(), target.ID)
	if err != nil {
		return fmt.Errorf("delete track: %w", err)
	}
	if removal != nil && !removal.Success {
		return errors.New("the server reported that the track was not deleted")
	}

	removed := 0
	if removal != nil {
		removed = len(removal.RemovedTrackIDs)
	}
	printer.Raw("")
	printer.Success("Track deleted")
	printer.Field("track", trackLabel(target))
	printer.Field("removed", strconv.Itoa(removed))
	printer.Field("file", "deleted from disk")
	return nil
}

func resolveTrackTarget(cmd *cobra.Command, printer *output.Printer, client *api.Client, argument string, byID bool, criteria tracks.Criteria) (tracks.Track, error) {
	if byID {
		printer.Step("Using track id %s", argument)
		return tracks.Track{ID: argument}, nil
	}

	printer.Step("Looking up track %q in the library", argument)
	found, err := client.SearchLibraryTracks(cmd.Context(), argument)
	if err != nil {
		return tracks.Track{}, fmt.Errorf("search tracks: %w", err)
	}

	resolved, err := tracks.Resolve(argument, criteria, toTracks(found))
	if err != nil {
		var ambiguous *tracks.AmbiguousError
		if errors.As(err, &ambiguous) {
			printer.Warn("%q matches %d tracks:", argument, len(ambiguous.Matches))
			for _, match := range ambiguous.Matches {
				printer.Info("%s  %s", match.ID, trackCandidateLabel(match))
			}
			return tracks.Track{}, errors.New(ambiguityHint(ambiguous.Matches))
		}
		return tracks.Track{}, err
	}
	return resolved, nil
}

// ambiguityHint points at the flag that can actually break the tie. --artist and
// --album are useless when every match sits on the same album.
func ambiguityHint(matches []tracks.Track) string {
	if shareAlbum(matches) {
		return "refusing to guess: these matches share the album — re-run with --track (and --disc if the number repeats across discs), or pass --id"
	}
	return "refusing to guess: narrow the search with --artist/--album, or pass --id"
}

func shareAlbum(matches []tracks.Track) bool {
	if len(matches) < 2 {
		return false
	}
	album := matcher.Normalize(matches[0].AlbumName)
	if album == "" {
		return false
	}
	for _, match := range matches[1:] {
		if matcher.Normalize(match.AlbumName) != album {
			return false
		}
	}
	return true
}

func confirmTrackDeletion(cmd *cobra.Command, printer *output.Printer, target tracks.Track) (bool, error) {
	stdinFile, isFile := cmd.InOrStdin().(*os.File)
	if !isFile || !output.IsTerminal(stdinFile) {
		return false, errors.New("refusing to delete without confirmation; re-run with --yes")
	}

	printer.Raw("")
	printer.Warn("this permanently deletes the track and its file from disk; DroppedNeedle has no catalog-only option for a single track")

	fmt.Fprintf(cmd.OutOrStdout(), "Delete %s? [y/N]: ", trackLabel(target))
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

func printTrackTarget(printer *output.Printer, target tracks.Track) {
	printer.Raw("")
	if strings.TrimSpace(target.Title) != "" {
		printer.Field("track", trackLabel(target))
	}
	printer.Field("id", target.ID)
	if strings.TrimSpace(target.AlbumName) != "" {
		printer.Field("album", target.AlbumName)
	}
	if position := trackPosition(target); position != "" {
		printer.Field("position", position)
	}
	if details := trackDetails(target); details != "" {
		printer.Field("details", details)
	}
}

func trackLabel(track tracks.Track) string {
	title := strings.TrimSpace(track.Title)
	if title == "" {
		return track.ID
	}
	if strings.TrimSpace(track.ArtistName) == "" {
		return title
	}
	return fmt.Sprintf("%s — %s", title, track.ArtistName)
}

// trackCandidateLabel renders one candidate of an ambiguous query. It carries
// everything that tells same-named tracks apart — album, disc/track number,
// format and duration — so the right id can be picked.
func trackCandidateLabel(track tracks.Track) string {
	label := trackLabel(track)
	if album := strings.TrimSpace(track.AlbumName); album != "" {
		label = fmt.Sprintf("%s (%s)", label, album)
	}

	parts := []string{label}
	if position := trackPosition(track); position != "" {
		parts = append(parts, position)
	}
	if format := strings.TrimSpace(track.Format); format != "" {
		parts = append(parts, format)
	}
	if duration := formatDuration(track.DurationSeconds); duration != "" {
		parts = append(parts, duration)
	}
	return strings.Join(parts, " · ")
}

// trackPosition renders the disc/track numbers, omitting unknowns and the
// implicit first disc.
func trackPosition(track tracks.Track) string {
	switch {
	case track.DiscNumber > 1 && track.TrackNumber > 0:
		return fmt.Sprintf("disc %d track %d", track.DiscNumber, track.TrackNumber)
	case track.TrackNumber > 0:
		return fmt.Sprintf("track %d", track.TrackNumber)
	case track.DiscNumber > 1:
		return fmt.Sprintf("disc %d", track.DiscNumber)
	default:
		return ""
	}
}

func trackDetails(track tracks.Track) string {
	parts := make([]string, 0, 2)
	if track.Format != "" {
		parts = append(parts, track.Format)
	}
	if duration := formatDuration(track.DurationSeconds); duration != "" {
		parts = append(parts, duration)
	}
	return strings.Join(parts, " · ")
}

func formatDuration(seconds float64) string {
	total := int(seconds + 0.5)
	if total <= 0 {
		return ""
	}
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}

func toTrack(track api.LibraryTrack) tracks.Track {
	return tracks.Track{
		ID:              track.ID,
		Title:           track.Title,
		ArtistName:      track.ArtistName,
		AlbumName:       track.AlbumTitle,
		TrackNumber:     track.TrackNumber,
		DiscNumber:      track.DiscNumber,
		DurationSeconds: track.DurationSeconds,
		Format:          track.Format,
	}
}

func toTracks(items []api.LibraryTrack) []tracks.Track {
	result := make([]tracks.Track, len(items))
	for i, item := range items {
		result[i] = toTrack(item)
	}
	return result
}

type trackRenameOptions struct {
	argument string
	newTitle string
	byID     bool
	artist   string
	album    string
	track    int
	disc     int
	mode     string
	yes      bool
	dryRun   bool
}

func newTrackRenameCommand(opts *globalOptions) *cobra.Command {
	input := trackRenameOptions{}

	cmd := &cobra.Command{
		Use:   "rename <track> <new-title>",
		Short: "Rename a track in the library",
		Long: `Rename a single track in the DroppedNeedle library.

Only the track's title changes: the title field is track-scoped, so the rest of
the album is untouched. Use "album rename" to rename the album itself.

The track is looked up by name and must match exactly one library track; use
--artist, --album, --track and/or --disc to narrow the search, or --id for an
exact track id.

DroppedNeedle stages metadata edits: a preview is created and its state is
checked, and only then is it applied. Applying writes the new title into the
track's audio file tags through the library management pipeline, so the change is
confirmed interactively unless --yes is given.

Use --dry-run to stage and validate the preview and then discard it without
queuing any work. The album's MusicBrainz identity must have been accepted
first.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			input.argument = args[0]
			input.newTitle = args[1]
			return runTrackRename(cmd, opts, input)
		},
	}

	cmd.Flags().BoolVar(&input.byID, "id", false, "treat the first argument as an exact track id and skip the library lookup")
	cmd.Flags().StringVar(&input.artist, "artist", "", "restrict the track lookup to this artist")
	cmd.Flags().StringVar(&input.album, "album", "", "restrict the track lookup to this album")
	cmd.Flags().IntVar(&input.track, "track", 0, "restrict the track lookup to this track number")
	cmd.Flags().IntVar(&input.disc, "disc", 0, "restrict the track lookup to this disc number")
	cmd.Flags().StringVar(&input.mode, "mode", renameModeSaveOverride,
		"how the new title is stored: save-override (future management runs keep it) or write-once")
	cmd.Flags().BoolVarP(&input.yes, "yes", "y", false, "do not ask for confirmation")
	cmd.Flags().BoolVar(&input.dryRun, "dry-run", false, "stage and validate the preview, then discard it without applying")

	return cmd
}

func runTrackRename(cmd *cobra.Command, opts *globalOptions, input trackRenameOptions) error {
	printer := opts.printer()

	title := strings.TrimSpace(input.newTitle)
	if title == "" {
		return errors.New("the new track title is empty")
	}
	if err := validateTrackFilter(input.byID, input.artist, input.album, input.track, input.disc); err != nil {
		return err
	}
	if _, err := tagEditMode(input.mode); err != nil {
		return err
	}

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

	target, err := resolveTrackTarget(cmd, printer, client, input.argument, input.byID, tracks.Criteria{
		Artist: input.artist,
		Album:  input.album,
		Track:  input.track,
		Disc:   input.disc,
	})
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("track %s", trackLabel(target))
	editor, err := loadTagEditor(cmd, printer, client, subject, target.ID)
	if err != nil {
		return err
	}
	field, ok := editor.Field(tagFieldTitle)
	if !ok {
		return errors.New("the tag editor does not expose a title field")
	}
	current := strings.TrimSpace(field.StringValue())
	if current == title {
		printer.Raw("")
		printer.Success("Track is already titled %q — nothing to do", title)
		return nil
	}

	return editTagField(cmd, printer, client, tagRename{
		Editor:  editor,
		Label:   "track",
		Subject: trackLabel(target),
		ID:      target.ID,
		Field:   tagFieldTitle,
		Current: current,
		New:     title,
		Mode:    input.mode,
		Warning: "applying this writes the new title into the tags of this track's audio file",
	}, input.dryRun, input.yes)
}
