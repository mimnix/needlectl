package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mimnix/needlectl/internal/api"
	"github.com/mimnix/needlectl/internal/output"
	"github.com/spf13/cobra"
)

// Tag editor modes accepted on the command line.
const (
	renameModeSaveOverride = "save-override"
	renameModeWriteOnce    = "write-once"
)

// Tag editor fields needlectl can rename. "title" is track-scoped; "album" and
// the other album fields are album-scoped.
const (
	tagFieldAlbum = "album"
	tagFieldTitle = "title"
)

// tagRename is a single-field rename staged through the DroppedNeedle tag
// editor and applied through the library management pipeline.
type tagRename struct {
	Editor  *api.TagEditor
	Label   string // field noun used in the summary: "album" or "track"
	Subject string // human label of the thing being renamed
	ID      string // album id or local track id
	Field   string // tag editor field to change
	Current string
	New     string
	Mode    string // CLI mode label
	Warning string // printed before the confirmation prompt
}

// loadTagEditor reads the tag editor of a library track. DroppedNeedle refuses
// to stage metadata edits until the album's MusicBrainz identity has been
// accepted, so that case is reported with the album's identity state.
func loadTagEditor(cmd *cobra.Command, printer *output.Printer, client *api.Client, subject, trackID string) (*api.TagEditor, error) {
	printer.Step("Reading the tag editor for %s", subject)
	editor, err := client.GetTagEditor(cmd.Context(), trackID)
	if err != nil {
		return nil, fmt.Errorf("read tag editor: %w", err)
	}
	if editor.AcceptedIdentity {
		return editor, nil
	}

	detail, detailErr := client.GetLibraryAlbum(cmd.Context(), editor.LocalAlbumID)
	if detailErr != nil {
		printer.Warn("could not read the album identity state: %v", detailErr)
		detail = nil
	}
	return nil, renameRefusedError(subject, editor, detail)
}

// editTagField stages a rename, verifies the staged preview, confirms it and
// applies it. Nothing is applied on --dry-run, on a refusal, or when the
// preview comes back stale, expired or not ready.
func editTagField(cmd *cobra.Command, printer *output.Printer, client *api.Client, rename tagRename, dryRun, yes bool) error {
	mode, err := tagEditMode(rename.Mode)
	if err != nil {
		return err
	}

	idempotencyKey, err := api.NewIdempotencyKey()
	if err != nil {
		return err
	}
	printer.Step("Staging the rename %q → %q (%s)", rename.Current, rename.New, rename.Mode)
	ref, err := client.CreateTagEditPreview(cmd.Context(), api.TagEditPreviewRequest{
		LocalTrackID:             rename.Editor.LocalTrackID,
		Mode:                     mode,
		ExpectedSettingsRevision: rename.Editor.SettingsRevision,
		ExpectedPolicyRevision:   rename.Editor.PolicyRevision,
		Fields:                   []api.TagEditField{{FieldName: rename.Field, Value: rename.New}},
		IdempotencyKey:           idempotencyKey,
	})
	if err != nil {
		return fmt.Errorf("stage rename: %w", err)
	}

	printTagRenamePreview(printer, rename, ref)

	preview, err := client.GetManagementPreview(cmd.Context(), ref.JobID)
	if err != nil {
		return fmt.Errorf("read staged preview: %w", err)
	}
	preview, err = waitForManagementPreview(cmd.Context(), client, ref.JobID, preview)
	if err != nil {
		return err
	}
	if blocker := previewBlocker(preview); blocker != nil {
		discardPreview(cmd, client, printer, ref.JobID, preview)
		return blocker
	}

	if dryRun {
		discardPreview(cmd, client, printer, ref.JobID, preview)
		printer.Raw("")
		printer.Success("Dry run complete — the preview was discarded and nothing was applied")
		return nil
	}

	if !yes {
		confirmed, err := confirmTagRename(cmd, printer, rename)
		if err != nil {
			return err
		}
		if !confirmed {
			discardPreview(cmd, client, printer, ref.JobID, preview)
			printer.Warn("aborted — nothing was renamed")
			return nil
		}
	}

	applyKey, err := api.NewIdempotencyKey()
	if err != nil {
		return err
	}
	operation, err := client.ApplyManagementPreview(cmd.Context(), ref.JobID, api.ApplyPreviewRequest{
		PreviewToken:                 ref.PreviewToken,
		ExpectedOperationRowRevision: preview.OperationRowRevision,
		IdempotencyKey:               applyKey,
		Confirmation:                 true,
	})
	if err != nil {
		return fmt.Errorf("apply rename: %w", err)
	}

	printer.Raw("")
	printer.Success("Rename queued")
	printer.Field(rename.Label, rename.Subject)
	if rename.ID != "" {
		printer.Field("id", rename.ID)
	}
	printer.Field("from", rename.Current)
	printer.Field("to", rename.New)
	printer.Field("mode", rename.Mode)
	if operation != nil && operation.ID != "" {
		printer.Field("operation", operation.ID)
	}
	printer.Info("the library management pipeline applies this in the background")
	return nil
}

// tagEditMode maps the CLI flag to the mode the API expects.
func tagEditMode(flag string) (string, error) {
	switch flag {
	case renameModeSaveOverride:
		return api.TagEditModeSaveOverride, nil
	case renameModeWriteOnce:
		return api.TagEditModeWriteOnce, nil
	default:
		return "", fmt.Errorf("unknown mode %q (use %s or %s)", flag, renameModeSaveOverride, renameModeWriteOnce)
	}
}

// previewWaitTimeout bounds how long a staged preview may take to become
// confirmable. DroppedNeedle v3 stages previews asynchronously, so a fresh
// preview is usually "queued" or "running" for a moment.
const previewWaitTimeout = 60 * time.Second

// waitForManagementPreview polls a staged preview until it is ready to confirm
// or reached a terminal state the caller can report.
func waitForManagementPreview(ctx context.Context, client *api.Client, jobID string, preview *api.ManagementPreview) (*api.ManagementPreview, error) {
	deadline := time.Now().Add(previewWaitTimeout)
	for {
		if preview == nil {
			return nil, errors.New("the server did not return the staged preview")
		}
		switch preview.State {
		case "queued", "running", "paused":
			// Still staging: keep waiting.
		default:
			return preview, nil
		}
		if preview.ReadyForConfirmation || preview.Expired || preview.Stale {
			return preview, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("the staged preview did not become ready within %s (state %s)", previewWaitTimeout, preview.State)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
		next, err := client.GetManagementPreview(ctx, jobID)
		if err != nil {
			return nil, fmt.Errorf("read staged preview: %w", err)
		}
		preview = next
	}
}

// previewBlocker explains why a staged preview cannot be applied yet, or nil
// when it is ready.
func previewBlocker(preview *api.ManagementPreview) error {
	if preview == nil {
		return errors.New("the server did not return the staged preview")
	}
	if preview.Expired {
		return errors.New("the staged preview expired — run the command again")
	}
	if preview.Stale {
		reason := strings.Join(preview.StaleReasons, "; ")
		if reason == "" {
			reason = "the metadata changed while the preview was staged"
		}
		return fmt.Errorf("the staged preview is stale: %s", reason)
	}
	if len(preview.OperationRowRevision) == 0 {
		return errors.New("the staged preview cannot be applied (the server did not return a revision to confirm)")
	}
	if !preview.ReadyForConfirmation {
		state := preview.State
		if state == "" {
			state = "not ready"
		}
		return fmt.Errorf("the staged preview is not ready to apply (%s)", state)
	}
	return nil
}

// renameRefusedError explains why the tag editor refuses metadata edits. The
// album's identity state is included when it could be read, because that names
// the review the user has to resolve first.
func renameRefusedError(subject string, editor *api.TagEditor, detail *api.LibraryAlbum) error {
	reason := "IDENTITY_NOT_ACCEPTED"
	if editor != nil && editor.IdentityReason != nil && strings.TrimSpace(*editor.IdentityReason) != "" {
		reason = strings.TrimSpace(*editor.IdentityReason)
	}
	if detail != nil {
		if summary := identitySummary(*detail); summary != "" {
			reason = reason + " · " + summary
		}
	}
	return fmt.Errorf("%s cannot be renamed: its MusicBrainz identity has not been accepted (%s); resolve the album's identity in DroppedNeedle first, then re-run this command",
		subject, reason)
}

// identitySummary renders the album identity fields that explain a refusal.
func identitySummary(album api.LibraryAlbum) string {
	parts := make([]string, 0, 3)
	if album.IdentityState != "" {
		parts = append(parts, "state "+album.IdentityState)
	}
	if album.ManagementHold != nil && strings.TrimSpace(album.ManagementHold.Kind) != "" {
		parts = append(parts, "management "+strings.TrimSpace(album.ManagementHold.Kind))
	}
	if album.PendingReviewID != nil && strings.TrimSpace(*album.PendingReviewID) != "" {
		parts = append(parts, "review "+strings.TrimSpace(*album.PendingReviewID))
	}
	return strings.Join(parts, " · ")
}

func printTagRenamePreview(printer *output.Printer, rename tagRename, ref *api.PreviewRef) {
	printer.Raw("")
	printer.Field(rename.Label, rename.Subject)
	if rename.ID != "" {
		printer.Field("id", rename.ID)
	}
	printer.Field("from", rename.Current)
	printer.Field("to", rename.New)
	printer.Field("mode", rename.Mode)
	if ref != nil {
		printer.Field("preview", ref.JobID)
	}
}

func discardPreview(cmd *cobra.Command, client *api.Client, printer *output.Printer, jobID string, preview *api.ManagementPreview) {
	revision := json.RawMessage(nil)
	if preview != nil {
		revision = preview.OperationRowRevision
	}
	if err := client.DiscardManagementPreview(cmd.Context(), jobID, revision); err != nil {
		printer.Warn("could not discard the preview job %s: %v", jobID, err)
	}
}

func confirmTagRename(cmd *cobra.Command, printer *output.Printer, rename tagRename) (bool, error) {
	stdinFile, isFile := cmd.InOrStdin().(*os.File)
	if !isFile || !output.IsTerminal(stdinFile) {
		return false, errors.New("refusing to rename without confirmation; re-run with --yes")
	}

	printer.Raw("")
	printer.Warn("%s", rename.Warning)

	fmt.Fprintf(cmd.OutOrStdout(), "Rename %q to %q? [y/N]: ", rename.Current, rename.New)
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
