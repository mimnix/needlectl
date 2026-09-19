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

	"github.com/mimmo/needlectl/internal/api"
	"github.com/mimmo/needlectl/internal/output"
	"github.com/spf13/cobra"
)

type importTracksOptions struct {
	mb      string
	wait    bool
	timeout time.Duration
	dryRun  bool
}

func newImportTracksCommand(opts *globalOptions) *cobra.Command {
	input := importTracksOptions{}

	cmd := &cobra.Command{
		Use:   "tracks <path>...",
		Short: "Upload tracks and match them to a MusicBrainz release",
		Long: `Upload audio tracks to DroppedNeedle's drop import.

DroppedNeedle always identifies the files itself first. With --mb, the entries it
could not identify ("needs_review") are then matched to the given release group.
Entries DroppedNeedle imported on its own are left untouched: only the server can
tell whether such an entry can still be re-matched, which is why the automatic
match is never overwritten here.

The reference accepts a MusicBrainz release URL, a release-group URL or a bare
MBID. A release URL is resolved to its release group first.

Directories are walked recursively; hidden files and directories are skipped.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImportTracks(cmd, opts, args, input)
		},
	}

	cmd.Flags().StringVar(&input.mb, "mb", "", "MusicBrainz release or release-group URL/MBID to match unidentified entries against")
	cmd.Flags().BoolVar(&input.wait, "wait", true, "wait for DroppedNeedle to finish identifying the uploaded files")
	cmd.Flags().DurationVar(&input.timeout, "timeout", 5*time.Minute, "how long to wait for identification")
	cmd.Flags().BoolVar(&input.dryRun, "dry-run", false, "show what would be uploaded and matched without doing it")

	return cmd
}

func runImportTracks(cmd *cobra.Command, opts *globalOptions, paths []string, input importTracksOptions) error {
	printer := opts.printer()

	files, err := collectUploadFiles(paths)
	if err != nil {
		return err
	}

	var releaseGroup string
	if strings.TrimSpace(input.mb) != "" {
		releaseGroup, err = resolveReleaseGroup(cmd.Context(), printer, input.mb)
		if err != nil {
			return err
		}
	}

	if input.dryRun {
		return printImportTracksDryRun(printer, files, releaseGroup)
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

	if releaseGroup == "" {
		if hasUnmatchedItems(job.Items) {
			printer.Warn("some entries still need a manual match; re-run with --mb or use 'needlectl import match'")
		}
		return nil
	}

	printer.Step("Matching to release group %s", releaseGroup)
	result := matchImportItems(cmd.Context(), printer, client, job.Items, releaseGroup, false)
	printMatchSummary(printer, result)

	if len(result.Failures) > 0 {
		return fmt.Errorf("%s could not be matched", output.CountLabel(len(result.Failures), "entry"))
	}
	return nil
}

// printImportTracksDryRun reports what an upload would send, without sending it.
func printImportTracksDryRun(printer *output.Printer, files []api.UploadFile, releaseGroup string) error {
	printer.Step("Dry run: %s would be uploaded", output.CountLabel(len(files), "file"))
	for _, file := range files {
		printer.Info("%s", file.Path)
	}
	if releaseGroup == "" {
		printer.Info("without --mb, unidentified entries would be left for 'needlectl import match'")
	} else {
		printer.Info("unidentified entries would be matched to release group %s", releaseGroup)
	}
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
			printer.Warn("identification is still running; check it later with 'needlectl import jobs'")
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
