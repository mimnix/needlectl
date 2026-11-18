package cmd

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mimnix/needlectl/internal/api"
	"github.com/mimnix/needlectl/internal/covers"
	"github.com/mimnix/needlectl/internal/output"
	"github.com/spf13/cobra"
)

// coverRepairPause keeps a bulk repair below the server's per-second request
// limit: one replacement is two requests.
const coverRepairPause = 100 * time.Millisecond

type playlistRepairCoversOptions struct {
	argument     string
	byID         bool
	allPlaylists bool
	dryRun       bool
}

func newPlaylistRepairCoversCommand(opts *globalOptions) *cobra.Command {
	input := playlistRepairCoversOptions{}

	cmd := &cobra.Command{
		Use:   "repair-covers [playlist]",
		Short: "Repair the album covers of a playlist's tracks",
		Long: `Repair the artwork shown on a playlist's tracks.

DroppedNeedle stores a cover URL on each playlist entry and never derives one
from the linked library file, so an entry added while the album had no artwork
(or linked with an older needlectl, which used the local album id) keeps showing
no cover. This command compares each linked entry with its album's MusicBrainz
release group and adds the correct artwork URL.

The API cannot patch cover_url — only source_type and available_sources are
writable — so an entry that needs repair is re-added at the same position and
the old entry removed, which preserves the playlist order. Entries that are not
linked to the local library are counted and left untouched.

Use --all-playlists to repair every playlist you own.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				input.argument = args[0]
			}
			return runPlaylistRepairCovers(cmd, opts, input)
		},
	}

	cmd.Flags().BoolVar(&input.byID, "id", false, "treat the argument as an exact playlist id")
	cmd.Flags().BoolVar(&input.allPlaylists, "all-playlists", false, "repair every playlist you own")
	cmd.Flags().BoolVar(&input.dryRun, "dry-run", false, "show what would be repaired without modifying any playlist")

	return cmd
}

func runPlaylistRepairCovers(cmd *cobra.Command, opts *globalOptions, input playlistRepairCoversOptions) error {
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

	target := covers.Target{Name: input.argument}
	if input.byID {
		target = covers.Target{ID: input.argument}
	}

	runner := &covers.Runner{
		API:    client,
		Out:    printer,
		DryRun: input.dryRun,
		All:    input.allPlaylists,
		Pause:  coverRepairPause,
	}
	results, err := runner.Run(cmd.Context(), target)
	if err != nil {
		var ambiguous *covers.AmbiguousPlaylistError
		if errors.As(err, &ambiguous) {
			printer.Warn("%q matches %d playlists:", ambiguous.Name, len(ambiguous.Matches))
			for _, match := range ambiguous.Matches {
				printer.Info("%s  %s", match.ID, match.Name)
			}
			return errors.New("refusing to guess: pass the playlist id with --id")
		}
		return err
	}

	printCoverRepairSummary(printer, results, input.dryRun)
	return nil
}

// validate rejects flag combinations that would silently ignore part of the
// request.
func (i playlistRepairCoversOptions) validate() error {
	if i.allPlaylists {
		if strings.TrimSpace(i.argument) != "" || i.byID {
			return errors.New("--all-playlists cannot be combined with a playlist argument or --id")
		}
		return nil
	}
	if strings.TrimSpace(i.argument) == "" {
		return errors.New("give a playlist name, a playlist id with --id, or --all-playlists")
	}
	return nil
}

func printCoverRepairSummary(printer *output.Printer, results []covers.Result, dryRun bool) {
	printer.Raw("")
	if dryRun {
		printer.Success("Dry run complete — no changes were made")
	} else {
		printer.Success("Cover repair complete")
	}

	if len(results) == 1 {
		printCoverRepairResult(printer, results[0], dryRun)
		return
	}

	repaired := 0
	playlists := 0
	for _, result := range results {
		if result.Total == 0 {
			continue
		}
		playlists++
		repaired += result.Repaired
		printer.Field(result.PlaylistName, coverRepairCountLabel(result, dryRun))
	}
	printer.Field("playlists", strconv.Itoa(playlists))
	label := "repaired"
	if dryRun {
		label = "to repair"
	}
	printer.Field(label, strconv.Itoa(repaired))
}

func printCoverRepairResult(printer *output.Printer, result covers.Result, dryRun bool) {
	printer.Field("playlist", result.PlaylistName)
	if result.PlaylistID != "" {
		printer.Field("id", result.PlaylistID)
	}
	printer.Field("library", output.CountLabel(result.LibrarySize, "track"))
	printer.Field("tracks", strconv.Itoa(result.Total))
	label := "repaired"
	if dryRun {
		label = "to repair"
	}
	printer.Field(label, strconv.Itoa(result.Repaired))
	printer.Field("unchanged", strconv.Itoa(result.Unchanged))
	if result.NoLibrary > 0 {
		printer.Field("not linked", strconv.Itoa(result.NoLibrary))
	}
	if result.NoReleaseGroup > 0 {
		printer.Field("no album", strconv.Itoa(result.NoReleaseGroup))
	}
}

func coverRepairCountLabel(result covers.Result, dryRun bool) string {
	if result.Total == 0 {
		return "empty"
	}
	label := output.CountLabel(result.Repaired, "entry") + " repaired"
	if dryRun {
		label = output.CountLabel(result.Repaired, "entry") + " to repair"
	}
	return fmt.Sprintf("%s · %d unchanged", label, result.Unchanged)
}
