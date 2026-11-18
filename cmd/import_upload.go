package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mimnix/needlectl/internal/api"
	"github.com/mimnix/needlectl/internal/output"
	"github.com/spf13/cobra"
)

type importUploadOptions struct {
	wait    bool
	timeout time.Duration
	dryRun  bool
}

func newImportUploadCommand(opts *globalOptions) *cobra.Command {
	input := importUploadOptions{}

	cmd := &cobra.Command{
		Use:   "upload <path>...",
		Short: "Upload files to DroppedNeedle's drop import",
		Long: `Upload audio files to DroppedNeedle's drop import.

DroppedNeedle identifies the files itself and reports how each group ended up.
Entries it cannot identify are left in the "needs_review" state; DroppedNeedle v3
dropped the API for resolving them, so they have to be decided in its web UI.

Directories are walked recursively; hidden files and directories are skipped.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImportUpload(cmd, opts, args, input)
		},
	}

	cmd.Flags().BoolVar(&input.wait, "wait", true, "wait for DroppedNeedle to finish identifying the uploaded files")
	cmd.Flags().DurationVar(&input.timeout, "timeout", 5*time.Minute, "how long to wait for identification")
	cmd.Flags().BoolVar(&input.dryRun, "dry-run", false, "show what would be uploaded without doing it")

	return cmd
}

func runImportUpload(cmd *cobra.Command, opts *globalOptions, paths []string, input importUploadOptions) error {
	printer := opts.printer()

	files, err := collectUploadFiles(paths)
	if err != nil {
		return err
	}

	if input.dryRun {
		return printImportUploadDryRun(printer, files)
	}

	cfg, err := opts.loadConfig(printer)
	if err != nil {
		return err
	}

	printer.Step("Uploading %s to %s", output.CountLabel(len(files), "file"), strings.TrimRight(cfg.Server, "/"))
	client, err := api.New(cfg.Server, cfg.InsecureSkipVerify)
	if err != nil {
		return err
	}
	if _, err := client.Login(cmd.Context(), cfg.Username, cfg.Password); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	job, err := client.UploadImportFiles(cmd.Context(), files)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	printer.Success("Uploaded as job %s from %s", importJobLabel(job.ID), job.UploadName)

	if input.wait {
		printer.Step("Waiting for DroppedNeedle to identify the files")
		job, err = waitForImportJob(cmd.Context(), printer, client, job.ID, input.timeout)
		if err != nil {
			return err
		}
	}

	printImportItems(printer, job.Items)

	if needsReviewItems(job.Items) > 0 {
		printer.Warn("%s still need review; DroppedNeedle v3 has no API to resolve them, use its web UI",
			output.CountLabel(needsReviewItems(job.Items), "entry"))
	}
	return nil
}

// printImportUploadDryRun reports what an upload would send, without sending it.
func printImportUploadDryRun(printer *output.Printer, files []api.UploadFile) error {
	printer.Step("Dry run: %s would be uploaded", output.CountLabel(len(files), "file"))
	for _, file := range files {
		printer.Info("%s", file.Path)
	}
	printer.Info("entries DroppedNeedle cannot identify would be left for review in its web UI")
	printer.Success("Dry run complete — nothing was uploaded")
	return nil
}

// collectUploadFiles expands the given paths into the files to upload. Regular
// files are taken as they are, directories are walked recursively.
func collectUploadFiles(paths []string) ([]api.UploadFile, error) {
	var files []api.UploadFile

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("cannot read upload path: %w", err)
		}

		if !info.IsDir() {
			files = append(files, api.UploadFile{Name: filepath.Base(path), Path: path})
			continue
		}

		walkErr := filepath.WalkDir(path, func(entry string, dirEntry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if strings.HasPrefix(dirEntry.Name(), ".") && entry != path {
				if dirEntry.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if dirEntry.IsDir() || !dirEntry.Type().IsRegular() {
				return nil
			}
			files = append(files, api.UploadFile{Name: dirEntry.Name(), Path: entry})
			return nil
		})
		if walkErr != nil {
			return nil, fmt.Errorf("cannot walk %s: %w", path, walkErr)
		}
	}

	if len(files) == 0 {
		return nil, errors.New("no files to upload")
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// waitForImportJob polls the job until every item settled or the timeout passes.
// Running out of time is not an error: the upload is already queued server side.
func waitForImportJob(ctx context.Context, printer *output.Printer, client *api.Client, jobID string, timeout time.Duration) (*api.ImportJob, error) {
	deadline := time.Now().Add(timeout)

	for {
		job, err := client.GetImportJob(ctx, jobID)
		if err != nil {
			return nil, err
		}
		if job.Settled() {
			return job, nil
		}
		if time.Now().After(deadline) {
			printer.Warn("identification is still running; check it later with 'needlectl import list'")
			return job, nil
		}
		printer.Debug("job %s: %s", importJobLabel(job.ID), settledLabel(job.Items))

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// needsReviewItems counts the entries that still wait for a decision.
func needsReviewItems(items []api.ImportItem) int {
	count := 0
	for _, item := range items {
		if item.NeedsReview() {
			count++
		}
	}
	return count
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
