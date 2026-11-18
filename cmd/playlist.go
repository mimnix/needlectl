package cmd

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/mimnix/needlectl/internal/api"
	"github.com/mimnix/needlectl/internal/output"
	"github.com/mimnix/needlectl/internal/relink"
	"github.com/spf13/cobra"
)

func newPlaylistCommand(opts *globalOptions) *cobra.Command {
	playlist := &cobra.Command{
		Use:   "playlist",
		Short: "Inspect and manage playlists",
		RunE:  groupRunE,
	}
	playlist.AddCommand(newImportPlaylistCommand(opts))
	playlist.AddCommand(newPlaylistRelinkCommand(opts))
	playlist.AddCommand(newPlaylistRepairCoversCommand(opts))
	playlist.AddCommand(newPlaylistRequestCommand(opts))
	return playlist
}

type playlistRelinkOptions struct {
	argument string
	byID     bool
	resolve  bool
	dryRun   bool
}

func newPlaylistRelinkCommand(opts *globalOptions) *cobra.Command {
	input := playlistRelinkOptions{}

	cmd := &cobra.Command{
		Use:   "relink <playlist>",
		Short: "Link the unplayable tracks of a playlist to library files",
		Long: `Scan the tracks of a playlist and link the ones that have no source yet to
files of the native DroppedNeedle library.

Every entry is matched against the library by title, artist and album. Entries
that already play through a source are left untouched, so a working link is
never replaced; only entries with no source at all are relinked.

An unlinked entry cannot be patched in place (the API only accepts source_type
and available_sources), so the linked entry is inserted at the same position and
the old entry removed right after. Playlist order is preserved.

By default the server is also asked to resolve sources, which links tracks by
album MusicBrainz id and track number; pass --resolve=false to skip it.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input.argument = args[0]
			return runPlaylistRelink(cmd, opts, input)
		},
	}

	cmd.Flags().BoolVar(&input.byID, "id", false, "treat the argument as an exact playlist id")
	cmd.Flags().BoolVar(&input.resolve, "resolve", true, "ask the server to resolve sources after relinking")
	cmd.Flags().BoolVar(&input.dryRun, "dry-run", false, "show what would be relinked without modifying the playlist")

	return cmd
}

func runPlaylistRelink(cmd *cobra.Command, opts *globalOptions, input playlistRelinkOptions) error {
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

	target := relink.Target{Name: input.argument}
	if input.byID {
		target = relink.Target{ID: input.argument}
	}

	runner := &relink.Runner{
		API:     client,
		Out:     printer,
		Resolve: input.resolve,
		DryRun:  input.dryRun,
	}
	result, err := runner.Run(cmd.Context(), target)
	if err != nil {
		var ambiguous *relink.AmbiguousPlaylistError
		if errors.As(err, &ambiguous) {
			printer.Warn("%q matches %d playlists:", ambiguous.Name, len(ambiguous.Matches))
			for _, match := range ambiguous.Matches {
				printer.Info("%s  %s", match.ID, match.Name)
			}
			return errors.New("refusing to guess: pass the playlist id with --id")
		}
		return err
	}

	printRelinkSummary(printer, result, input.dryRun)
	return nil
}

func printRelinkSummary(printer *output.Printer, result relink.Result, dryRun bool) {
	printer.Raw("")
	if dryRun {
		printer.Success("Dry run complete — no changes were made")
	} else {
		printer.Success("Relink complete")
	}

	printer.Field("playlist", result.PlaylistName)
	if result.PlaylistID != "" {
		printer.Field("id", result.PlaylistID)
	}
	printer.Field("library", output.CountLabel(result.LibrarySize, "track"))
	printer.Field("tracks", strconv.Itoa(result.Total))
	printer.Field("playable", strconv.Itoa(result.Playable))
	printer.Field("relinked", strconv.Itoa(result.Relinked))
	printer.Field("unmatched", strconv.Itoa(result.Unmatched))
	if !dryRun {
		printer.Field("playable now", strconv.Itoa(result.PlayableAfter))
		printer.Field("unlinked", strconv.Itoa(result.StillUnlinked))
	}
}
