package cmd

import (
	"github.com/spf13/cobra"
)

// newImportCommand builds the "import" command group.
//
// It is a runnable group: bare "needlectl import foo.csv" keeps importing a CSV
// playlist exactly as before, while the subcommands cover uploading files and
// fixing the uploads DroppedNeedle could not identify.
func newImportCommand(opts *globalOptions) *cobra.Command {
	input := importOptions{}

	cmd := &cobra.Command{
		Use:   "import [playlist.csv]",
		Short: "Import playlists and files",
		Long: `Import playlists and files into DroppedNeedle.

Without a subcommand the command imports a CSV playlist, exactly like
"needlectl import playlist".

Subcommands:
  playlist <file.csv>   import a CSV playlist
  tracks <path>...      upload audio tracks and match them to a MusicBrainz release
  match <item-id>...    match uploads DroppedNeedle could not identify
  jobs                  list import jobs and their items`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			input.path = args[0]
			return runImport(cmd, opts, input)
		},
	}

	addPlaylistFlags(cmd, &input)
	cmd.AddCommand(
		newImportPlaylistCommand(opts),
		newImportTracksCommand(opts),
		newImportMatchCommand(opts),
		newImportJobsCommand(opts),
	)

	return cmd
}
