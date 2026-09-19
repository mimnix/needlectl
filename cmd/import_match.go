package cmd

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/mimmo/needlectl/internal/api"
	"github.com/mimmo/needlectl/internal/output"
	"github.com/spf13/cobra"
)

type importMatchOptions struct {
	mb     string
	all    bool
	dryRun bool
}

func newImportMatchCommand(opts *globalOptions) *cobra.Command {
	input := importMatchOptions{}

	cmd := &cobra.Command{
		Use:   "match <item-id>... | --all",
		Short: "Match import entries to a MusicBrainz release",
		Long: `Match entries of an upload to a MusicBrainz release group.

This is the manual step the DroppedNeedle UI offers when an upload lands in
"needs_review" because the album could not be worked out automatically. Use
"needlectl import jobs --status needs_review" to find those entries.

Only entries that are still waiting for review are matched. Entries DroppedNeedle
already imported are left alone, because only the server can say whether such an
entry can still be re-matched.

The reference accepts a MusicBrainz release URL, a release-group URL or a bare
MBID; a release URL is resolved to its release group first.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(input.mb) == "" {
				return errors.New("--mb is required")
			}
			if input.all && len(args) > 0 {
				return fmt.Errorf("use either --all or explicit item ids, not both")
			}
			if !input.all && len(args) == 0 {
				return fmt.Errorf("give at least one item id, or use --all")
			}
			for _, arg := range args {
				if _, err := strconv.Atoi(arg); err != nil {
					return fmt.Errorf("%q is not an import item id", arg)
				}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImportMatch(cmd, opts, args, input)
		},
	}

	cmd.Flags().StringVar(&input.mb, "mb", "", "MusicBrainz release or release-group URL/MBID to match against")
	cmd.Flags().BoolVar(&input.all, "all", false, "match every entry that is waiting for review")
	cmd.Flags().BoolVar(&input.dryRun, "dry-run", false, "show what would be matched without doing it")

	return cmd
}

func runImportMatch(cmd *cobra.Command, opts *globalOptions, args []string, input importMatchOptions) error {
	printer := opts.printer()

	releaseGroup, err := resolveReleaseGroup(cmd.Context(), printer, input.mb)
	if err != nil {
		return err
	}

	cfg, err := opts.loadConfig(printer)
	if err != nil {
		return err
	}

	client, err := api.New(cfg.Server, cfg.InsecureSkipVerify)
	if err != nil {
		return err
	}
	if _, err := client.Login(cmd.Context(), cfg.Username, cfg.Password); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	items, err := importMatchCandidates(cmd, client, args, input.all)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		printer.Info("nothing to match")
		return nil
	}

	result := matchImportItems(cmd.Context(), printer, client, items, releaseGroup, input.dryRun)
	printMatchSummary(printer, result)

	if len(result.Failures) > 0 {
		return fmt.Errorf("%s could not be matched", output.CountLabel(len(result.Failures), "entry"))
	}
	return nil
}

// importMatchCandidates collects the entries to match, either by id or from every
// job of the user, and always filters out the ones that no longer await review.
func importMatchCandidates(cmd *cobra.Command, client *api.Client, args []string, all bool) ([]api.ImportItem, error) {
	if !all {
		items := make([]api.ImportItem, 0, len(args))
		for _, arg := range args {
			id, err := strconv.Atoi(arg)
			if err != nil {
				return nil, fmt.Errorf("%q is not an import item id", arg)
			}
			items = append(items, api.ImportItem{ID: id, Status: api.ImportStatusNeedsReview})
		}
		return items, nil
	}

	jobs, err := client.ListImportJobs(cmd.Context(), false)
	if err != nil {
		return nil, err
	}
	var items []api.ImportItem
	for _, job := range jobs {
		for _, item := range job.Items {
			if item.NeedsReview() {
				items = append(items, item)
			}
		}
	}
	return items, nil
}

// matchResult summarises a matching run.
type matchResult struct {
	Matched  []api.ImportItem
	Skipped  int
	Failures []string
}

// matchImportItems matches every entry that is still waiting for review. Entries
// DroppedNeedle already imported are counted as skipped and never re-matched.
func matchImportItems(ctx context.Context, printer *output.Printer, client *api.Client, items []api.ImportItem, releaseGroupMBID string, dryRun bool) matchResult {
	result := matchResult{}

	for _, item := range items {
		if !item.NeedsReview() {
			result.Skipped++
			continue
		}
		if dryRun {
			printer.Info("would match item %d to %s", item.ID, releaseGroupMBID)
			result.Matched = append(result.Matched, item)
			continue
		}

		updated, err := client.MatchImportItem(ctx, item.ID, releaseGroupMBID)
		if err != nil {
			printer.Error("item %d: %v", item.ID, err)
			result.Failures = append(result.Failures, fmt.Sprintf("item %d: %v", item.ID, err))
			continue
		}

		printer.Success("item %d matched to %s", updated.ID, importItemLabel(*updated))
		result.Matched = append(result.Matched, *updated)
	}

	return result
}

func printMatchSummary(printer *output.Printer, result matchResult) {
	printer.Raw("")
	if len(result.Matched) == 0 && len(result.Failures) == 0 {
		printer.Info("no entry needed matching")
		return
	}

	printer.Success("Matched %s", output.CountLabel(len(result.Matched), "entry"))
	if result.Skipped > 0 {
		printer.Field("left alone", fmt.Sprintf("%d already imported", result.Skipped))
	}
	if len(result.Failures) > 0 {
		printer.Field("failed", strconv.Itoa(len(result.Failures)))
	}
}

// hasUnmatchedItems reports whether any entry still waits for a manual match.
func hasUnmatchedItems(items []api.ImportItem) bool {
	for _, item := range items {
		if item.NeedsReview() {
			return true
		}
	}
	return false
}

// settledLabel counts how many items already reached a final state.
func settledLabel(items []api.ImportItem) string {
	settled := 0
	for _, item := range items {
		if item.Settled() {
			settled++
		}
	}
	return fmt.Sprintf("%d of %d settled", settled, len(items))
}
