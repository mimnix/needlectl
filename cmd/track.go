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
	return track
}

type trackDeleteOptions struct {
	argument string
	byID     bool
	artist   string
	album    string
	yes      bool
	dryRun   bool
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

The track is looked up by name and must match exactly one library track; use
--artist and/or --album to narrow the search, or --id to pass an exact track id.

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
	cmd.Flags().BoolVarP(&input.yes, "yes", "y", false, "do not ask for confirmation")
	cmd.Flags().BoolVar(&input.dryRun, "dry-run", false, "show what would be deleted without deleting anything")

	return cmd
}

func runTrackDelete(cmd *cobra.Command, opts *globalOptions, input trackDeleteOptions) error {
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

	target, err := resolveTrackTarget(cmd, printer, client, input)
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

func resolveTrackTarget(cmd *cobra.Command, printer *output.Printer, client *api.Client, input trackDeleteOptions) (tracks.Track, error) {
	if input.byID {
		printer.Step("Using track id %s", input.argument)
		return tracks.Track{ID: input.argument}, nil
	}

	printer.Step("Looking up track %q in the library", input.argument)
	found, err := client.SearchLibraryTracks(cmd.Context(), input.argument)
	if err != nil {
		return tracks.Track{}, fmt.Errorf("search tracks: %w", err)
	}

	resolved, err := tracks.Resolve(input.argument, input.artist, input.album, toTracks(found))
	if err != nil {
		var ambiguous *tracks.AmbiguousError
		if errors.As(err, &ambiguous) {
			printer.Warn("%q matches %d tracks:", input.argument, len(ambiguous.Matches))
			for _, match := range ambiguous.Matches {
				printer.Info("%s  %s", match.ID, trackCandidateLabel(match))
			}
			return tracks.Track{}, errors.New("refusing to guess: narrow the search with --artist/--album or pass --id")
		}
		return tracks.Track{}, err
	}
	return resolved, nil
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

func trackCandidateLabel(track tracks.Track) string {
	if strings.TrimSpace(track.AlbumName) == "" {
		return trackLabel(track)
	}
	return fmt.Sprintf("%s (%s)", trackLabel(track), track.AlbumName)
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
