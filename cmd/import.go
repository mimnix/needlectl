package cmd

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mimmo/needlectl/internal/api"
	"github.com/mimmo/needlectl/internal/importer"
	"github.com/mimmo/needlectl/internal/output"
	"github.com/mimmo/needlectl/internal/playlist"
	"github.com/spf13/cobra"
)

type importOptions struct {
	path      string
	name      string
	dryRun    bool
	resolve   bool
	batchSize int
}

func newImportCommand(opts *globalOptions) *cobra.Command {
	input := importOptions{}

	cmd := &cobra.Command{
		Use:   "import <playlist.csv>",
		Short: "Import a CSV playlist into DroppedNeedle",
		Long: `Import a CSV playlist into a DroppedNeedle playlist.

Tracks already present in the playlist are left untouched, so the command is
idempotent and safe to re-run. Every row is matched against the DroppedNeedle
library: matching tracks are linked to their library file, and previously
unlinked entries whose track has since been added to the library are relinked.

Accepted columns (case-insensitive, order-independent):
  Track name / Title / Track   (required)
  Artist name / Artist         (required)
  Album / Album name
  Track number, Disc number, Duration (ms), Playlist name, ISRC, Spotify - id

The playlist name defaults to the CSV file name without its extension.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input.path = args[0]
			return runImport(cmd, opts, input)
		},
	}

	cmd.Flags().StringVar(&input.name, "name", "", "playlist name (default: the CSV file name without extension)")
	cmd.Flags().BoolVar(&input.dryRun, "dry-run", false, "show what would change without modifying the playlist")
	cmd.Flags().BoolVar(&input.resolve, "resolve", true, "resolve track sources after importing (links tracks that are now available)")
	cmd.Flags().IntVar(&input.batchSize, "batch-size", 100, "number of tracks sent per add request")

	return cmd
}

func runImport(cmd *cobra.Command, opts *globalOptions, input importOptions) error {
	printer := opts.printer()

	cfg, err := opts.loadConfig(printer)
	if err != nil {
		return err
	}

	printer.Step("Reading %s", input.path)
	rows, warnings, err := playlist.ParseFile(input.path)
	if err != nil {
		return err
	}
	for _, warning := range warnings {
		printer.Warn("%s:%d: %s", filepath.Base(input.path), warning.Line, warning.Message)
	}
	if len(rows) == 0 {
		return fmt.Errorf("no importable tracks found in %s", input.path)
	}
	printer.Info("%s, %d warning(s)", countLabel(len(rows), "track"), len(warnings))

	name := strings.TrimSpace(input.name)
	if name == "" {
		name = defaultPlaylistName(input.path)
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

	runner := &importer.Runner{
		API:            client,
		Out:            printer,
		BatchSize:      input.batchSize,
		ResolveSources: input.resolve,
		DryRun:         input.dryRun,
	}
	result, err := runner.Run(cmd.Context(), name, rows)
	if err != nil {
		return err
	}

	printSummary(printer, result, input.dryRun)
	return nil
}

func printSummary(printer *output.Printer, result importer.Result, dryRun bool) {
	printer.Raw("")
	if dryRun {
		printer.Success("Dry run complete — no changes were made")
	} else {
		printer.Success("Import complete")
	}

	printer.Field("playlist", result.PlaylistName)
	if result.PlaylistID != "" {
		printer.Field("id", result.PlaylistID)
	}
	printer.Field("library", countLabel(result.LibrarySize, "track"))
	printer.Field("added", fmt.Sprintf("%d (%d linked)", result.AddedLinked+result.AddedUnlinked, result.AddedLinked))
	printer.Field("refreshed", strconv.Itoa(result.Refreshed))
	printer.Field("unchanged", strconv.Itoa(result.Unchanged))
	if !dryRun {
		printer.Field("playable", strconv.Itoa(result.Playable))
		printer.Field("unlinked", strconv.Itoa(result.StillUnlinked))
	}
}

func defaultPlaylistName(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func countLabel(n int, singular string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", singular)
	}
	return fmt.Sprintf("%d %ss", n, singular)
}
