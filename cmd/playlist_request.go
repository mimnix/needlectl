package cmd

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mimmo/needlectl/internal/acquire"
	"github.com/mimmo/needlectl/internal/api"
	"github.com/mimmo/needlectl/internal/mbref"
	"github.com/mimmo/needlectl/internal/output"
	"github.com/spf13/cobra"
)

// acquirePause keeps a bulk acquisition below the server's request limit: each
// album costs one search and, when matched, one request.
const acquirePause = 150 * time.Millisecond

type playlistRequestOptions struct {
	argument string
	byID     bool
	albums   bool
	dryRun   bool
}

func newPlaylistRequestCommand(opts *globalOptions) *cobra.Command {
	input := playlistRequestOptions{}

	cmd := &cobra.Command{
		Use:   "request <playlist>",
		Short: "Request a playlist's missing tracks",
		Long: `Request the missing tracks of a playlist.

Playlist rows usually carry no MusicBrainz ids, so each unplayable entry is
grouped by artist and album and the album is resolved through the server's
MusicBrainz-backed search. Only an exact album title and artist match is used;
anything less certain is reported and left alone.

Two modes are available:

- per track (default): the album's MusicBrainz tracklist is read and the entry's
  title is matched to a recording, then one track request is sent. This fetches
  only that recording.
- --albums: one album request is sent for the release group. Use it when the
  whole album is wanted or when a track title does not match the tracklist.

Curators get each request dispatched at once; other users get requests awaiting
approval. Albums already requested are skipped in both modes.

Run "needlectl playlist relink" first: entries whose track is already in the
library are linked instead of downloaded.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input.argument = args[0]
			return runPlaylistRequest(cmd, opts, input)
		},
	}

	cmd.Flags().BoolVar(&input.byID, "id", false, "treat the argument as an exact playlist id")
	cmd.Flags().BoolVar(&input.albums, "albums", false, "request each album once instead of one recording per missing track")
	cmd.Flags().BoolVar(&input.dryRun, "dry-run", false, "resolve the tracks and show what would be requested without requesting it")

	return cmd
}

func runPlaylistRequest(cmd *cobra.Command, opts *globalOptions, input playlistRequestOptions) error {
	if strings.TrimSpace(input.argument) == "" {
		return errors.New("give a playlist name or, with --id, a playlist id")
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

	target := acquire.Target{Name: input.argument}
	if input.byID {
		target = acquire.Target{ID: input.argument}
	}

	mode := acquire.ModeTrack
	if input.albums {
		mode = acquire.ModeAlbum
	}
	runner := &acquire.Runner{
		API:    client,
		Tracks: mbref.NewResolver(),
		Out:    printer,
		DryRun: input.dryRun,
		Mode:   mode,
		Pause:  acquirePause,
	}
	result, err := runner.Run(cmd.Context(), target)
	if err != nil {
		var ambiguous *acquire.AmbiguousPlaylistError
		if errors.As(err, &ambiguous) {
			printer.Warn("%q matches %d playlists:", ambiguous.Name, len(ambiguous.Matches))
			for _, match := range ambiguous.Matches {
				printer.Info("%s  %s", match.ID, match.Name)
			}
			return errors.New("refusing to guess: pass the playlist id with --id")
		}
		return err
	}

	printRequestSummary(printer, result, input.dryRun)
	return nil
}

func printRequestSummary(printer *output.Printer, result acquire.Result, dryRun bool) {
	printer.Raw("")
	if dryRun {
		printer.Success("Dry run complete — nothing was requested")
	} else {
		printer.Success("Requests complete")
	}

	printer.Field("playlist", result.PlaylistName)
	if result.PlaylistID != "" {
		printer.Field("id", result.PlaylistID)
	}
	printer.Field("mode", result.Mode.String())
	printer.Field("tracks", strconv.Itoa(result.Total))
	printer.Field("not playable", strconv.Itoa(result.Missing))
	printer.Field("albums", strconv.Itoa(result.Albums))

	label := "requests"
	if dryRun {
		label = "to request"
	}
	printer.Field(label, strconv.Itoa(result.Requested))
	if result.AlreadyAsked > 0 {
		printer.Field("already asked", strconv.Itoa(result.AlreadyAsked))
	}
	if result.Unmatched > 0 {
		printer.Field("unmatched", output.CountLabel(result.Unmatched, "track"))
	}
	if result.Failed > 0 {
		printer.Field("failed", strconv.Itoa(result.Failed))
	}
	printer.Field("covered", output.CountLabel(result.Covered, "track"))

	if result.Missing == 0 {
		printer.Info("every entry is already playable")
	} else if result.Unmatched > 0 {
		message := "unmatched albums need a MusicBrainz id by hand"
		if result.Mode == acquire.ModeTrack {
			message = "unmatched tracks have no exact album or tracklist match"
		}
		printer.Info("%s; use --verbose to list them", message)
	}
}
