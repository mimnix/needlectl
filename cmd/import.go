package cmd

import (
	"github.com/spf13/cobra"
)

// newImportCommand builds the "import" command group. It is a resource: the
// drop-import batches created by uploading files, not the CSV playlist import
// (which belongs to "playlist import").
func newImportCommand(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Upload files and inspect import batches",
		RunE:  groupRunE,
		Long: `Work with DroppedNeedle's drop import.

"needlectl import upload" sends audio files to the drop import and
"needlectl import list" shows the batches it created. Importing a CSV playlist
is a playlist operation: use "needlectl playlist import".`,
	}

	cmd.AddCommand(
		newImportUploadCommand(opts),
		newImportListCommand(opts),
	)

	return cmd
}
