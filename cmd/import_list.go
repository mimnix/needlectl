package cmd

import (
	"fmt"
	"strings"

	"github.com/mimmo/needlectl/internal/api"
	"github.com/mimmo/needlectl/internal/output"
	"github.com/spf13/cobra"
)

type importListOptions struct {
	all    bool
	status string
}

func newImportListCommand(opts *globalOptions) *cobra.Command {
	input := importListOptions{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List import batches and their entries",
		Long: `List the drop import batches of the signed-in user.

Entries DroppedNeedle could not identify are shown as "needs_review". DroppedNeedle
v3 has no API to resolve them, so they have to be decided in its web UI.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImportList(cmd, opts, input)
		},
	}

	cmd.Flags().BoolVar(&input.all, "all", false, "list the batches of every user instead of only yours")
	cmd.Flags().StringVar(&input.status, "status", "", "only show entries with this status (for example needs_review)")

	return cmd
}

func runImportList(cmd *cobra.Command, opts *globalOptions, input importListOptions) error {
	printer := opts.printer()

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

	jobs, err := client.ListImportJobs(cmd.Context(), input.all)
	if err != nil {
		return err
	}

	shown := 0
	for _, job := range jobs {
		items := filterImportItems(job.Items, input.status)
		if len(items) == 0 {
			continue
		}
		shown++
		printImportJob(printer, job, items)
	}

	if shown == 0 {
		printer.Info("no import batches%s found", statusSuffix(input.status))
		return nil
	}

	printer.Raw("")
	printer.Success("%s, %s", output.CountLabel(shown, "job"), output.CountLabel(countImportItems(jobs, input.status), "entry"))
	return nil
}

// printImportJob renders one job and the entries selected from it.
func printImportJob(printer *output.Printer, job api.ImportJob, items []api.ImportItem) {
	printer.Raw("")
	printer.Field("job", importJobLabel(job.ID))
	printer.Field("uploaded", job.UploadName)
	if err := job.ErrorMessage(); err != "" {
		printer.Field("error", err)
	}

	for _, item := range items {
		printer.Info("item %d  %-12s  %s", item.ID, item.Status, importItemLabel(item))
		if message := item.Message(); message != "" {
			printer.Info("         %s", message)
		}
	}
}

// printImportItems renders the entries of a freshly uploaded job.
func printImportItems(printer *output.Printer, items []api.ImportItem) {
	for _, item := range items {
		printer.Info("item %d  %-12s  %s", item.ID, item.Status, importItemLabel(item))
		if message := item.Message(); message != "" {
			printer.Info("         %s", message)
		}
	}
}

// importItemLabel describes one entry for humans.
func importItemLabel(item api.ImportItem) string {
	parts := make([]string, 0, 2)
	if title := item.Title(); title != "" {
		parts = append(parts, title)
	}
	if artist := item.Artist(); artist != "" {
		parts = append(parts, artist)
	}
	if len(parts) == 0 {
		if item.FolderName != "" {
			return fmt.Sprintf("%s (%d/%d files)", item.FolderName, item.FilesImported, item.FilesTotal)
		}
		return fmt.Sprintf("%d/%d files", item.FilesImported, item.FilesTotal)
	}
	return fmt.Sprintf("%s (%d/%d files)", strings.Join(parts, " · "), item.FilesImported, item.FilesTotal)
}

// importJobLabel shortens a job id for display.
func importJobLabel(jobID string) string {
	if len(jobID) > 8 {
		return jobID[:8]
	}
	return jobID
}

// filterImportItems keeps the entries matching status; an empty status keeps all.
func filterImportItems(items []api.ImportItem, status string) []api.ImportItem {
	wanted := strings.TrimSpace(status)
	if wanted == "" {
		return items
	}

	filtered := make([]api.ImportItem, 0, len(items))
	for _, item := range items {
		if item.Status == wanted {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// countImportItems counts the entries selected across jobs.
func countImportItems(jobs []api.ImportJob, status string) int {
	total := 0
	for _, job := range jobs {
		total += len(filterImportItems(job.Items, status))
	}
	return total
}

// statusSuffix renders the " with status X" part of the empty-result message.
func statusSuffix(status string) string {
	trimmed := strings.TrimSpace(status)
	if trimmed == "" {
		return ""
	}
	return " with status " + trimmed
}
