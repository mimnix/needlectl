package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/mimmo/needlectl/internal/albums"
	"github.com/mimmo/needlectl/internal/api"
	"github.com/mimmo/needlectl/internal/output"
	"github.com/spf13/cobra"
)

func newAlbumCommand(opts *globalOptions) *cobra.Command {
	album := &cobra.Command{
		Use:   "album",
		Short: "Inspect and manage albums in the DroppedNeedle library",
	}
	album.AddCommand(newAlbumDeleteCommand(opts))
	return album
}

type albumDeleteOptions struct {
	argument   string
	byID       bool
	artist     string
	keepFiles  bool
	keepWanted bool
	yes        bool
	dryRun     bool
}

func newAlbumDeleteCommand(opts *globalOptions) *cobra.Command {
	input := albumDeleteOptions{}

	cmd := &cobra.Command{
		Use:   "delete <album>",
		Short: "Delete an album and all of its tracks",
		Long: `Delete an album from the DroppedNeedle library, cascading to every track that
belongs to it.

By default the audio files are deleted from disk as well; pass --keep-files to
only remove the album from the catalog. Unless --keep-wanted is given, the album
is also stopped from being downloaded again.

The album is looked up by name and must match exactly one library album; use
--artist to narrow the search, or --id to pass an exact album id.

Deletion is confirmed interactively unless --yes is given. In a non-interactive
session --yes is required.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input.argument = args[0]
			return runAlbumDelete(cmd, opts, input)
		},
	}

	cmd.Flags().BoolVar(&input.byID, "id", false, "treat the argument as an exact album id and skip the library lookup")
	cmd.Flags().StringVar(&input.artist, "artist", "", "restrict the album lookup to this artist")
	cmd.Flags().BoolVar(&input.keepFiles, "keep-files", false, "only remove the album from the catalog; keep the files on disk")
	cmd.Flags().BoolVar(&input.keepWanted, "keep-wanted", false, "keep the album queued for future automatic downloads")
	cmd.Flags().BoolVarP(&input.yes, "yes", "y", false, "do not ask for confirmation")
	cmd.Flags().BoolVar(&input.dryRun, "dry-run", false, "show what would be deleted without deleting anything")

	return cmd
}

func runAlbumDelete(cmd *cobra.Command, opts *globalOptions, input albumDeleteOptions) error {
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

	target, err := resolveAlbumTarget(cmd, printer, client, input)
	if err != nil {
		return err
	}
	printAlbumTarget(printer, target)

	deleteFiles := !input.keepFiles
	stopWanted := !input.keepWanted

	if input.dryRun {
		printer.Raw("")
		printer.Success("Dry run complete — nothing was deleted")
		printer.Field("files", fileAction(deleteFiles))
		printer.Field("wanted", wantedAction(stopWanted))
		return nil
	}

	if !input.yes {
		confirmed, err := confirmDeletion(cmd, printer, target, deleteFiles)
		if err != nil {
			return err
		}
		if !confirmed {
			printer.Warn("aborted — nothing was deleted")
			return nil
		}
	}

	printer.Step("Deleting album %s", albumLabel(target))
	removal, err := client.DeleteAlbum(cmd.Context(), target.ID, deleteFiles, stopWanted)
	if err != nil {
		return fmt.Errorf("delete album: %w", err)
	}
	if removal != nil && !removal.Success {
		return errors.New("the server reported that the album was not deleted")
	}

	removed := 0
	if removal != nil {
		removed = len(removal.RemovedTrackIDs)
	}
	printer.Raw("")
	printer.Success("Album deleted")
	printer.Field("album", albumLabel(target))
	printer.Field("tracks", strconv.Itoa(removed))
	printer.Field("files", fileAction(deleteFiles))
	printer.Field("wanted", wantedAction(stopWanted))
	return nil
}

func resolveAlbumTarget(cmd *cobra.Command, printer *output.Printer, client *api.Client, input albumDeleteOptions) (albums.Album, error) {
	if input.byID {
		printer.Step("Loading album %s", input.argument)
		detail, err := client.GetLibraryAlbum(cmd.Context(), input.argument)
		if err != nil {
			return albums.Album{}, fmt.Errorf("load album %q: %w", input.argument, err)
		}
		return toAlbum(*detail), nil
	}

	printer.Step("Looking up album %q in the library", input.argument)
	found, err := client.ListLibraryAlbums(cmd.Context(), input.argument)
	if err != nil {
		return albums.Album{}, fmt.Errorf("search albums: %w", err)
	}

	resolved, err := albums.Resolve(input.argument, input.artist, toAlbums(found))
	if err != nil {
		var ambiguous *albums.AmbiguousError
		if errors.As(err, &ambiguous) {
			printer.Warn("%q matches %d albums:", input.argument, len(ambiguous.Matches))
			for _, match := range ambiguous.Matches {
				printer.Info("%s  %s", match.ID, albumLabel(match))
			}
			return albums.Album{}, errors.New("refusing to guess: narrow the search with --artist or pass --id")
		}
		return albums.Album{}, err
	}
	return resolved, nil
}

func confirmDeletion(cmd *cobra.Command, printer *output.Printer, target albums.Album, deleteFiles bool) (bool, error) {
	stdinFile, isFile := cmd.InOrStdin().(*os.File)
	if !isFile || !output.IsTerminal(stdinFile) {
		return false, errors.New("refusing to delete without confirmation; re-run with --yes")
	}

	printer.Raw("")
	if deleteFiles {
		printer.Warn("this permanently deletes the album, its %s and the files on disk",
			output.CountLabel(target.TrackCount, "track"))
	} else {
		printer.Warn("this removes the album and its %s from the catalog (the files are kept)",
			output.CountLabel(target.TrackCount, "track"))
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Delete %s? [y/N]: ", albumLabel(target))
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

func printAlbumTarget(printer *output.Printer, target albums.Album) {
	printer.Raw("")
	printer.Field("album", albumLabel(target))
	printer.Field("id", target.ID)
	if target.TrackCount > 0 {
		printer.Field("tracks", strconv.Itoa(target.TrackCount))
	}
	if details := albumDetails(target); details != "" {
		printer.Field("details", details)
	}
}

func albumLabel(album albums.Album) string {
	if strings.TrimSpace(album.ArtistName) == "" {
		return album.Title
	}
	return fmt.Sprintf("%s — %s", album.Title, album.ArtistName)
}

func albumDetails(album albums.Album) string {
	parts := make([]string, 0, 2)
	if album.Format != "" {
		parts = append(parts, album.Format)
	}
	if album.Year > 0 {
		parts = append(parts, strconv.Itoa(album.Year))
	}
	return strings.Join(parts, " · ")
}

func fileAction(deleteFiles bool) string {
	if deleteFiles {
		return "delete from disk"
	}
	return "keep on disk"
}

func wantedAction(stopWanted bool) string {
	if stopWanted {
		return "stop future downloads"
	}
	return "keep wanted"
}

func toAlbum(album api.LibraryAlbum) albums.Album {
	result := albums.Album{
		ID:         album.ID,
		Title:      album.Title,
		ArtistName: album.ArtistName,
		TrackCount: album.TrackCount,
	}
	if album.Format != nil {
		result.Format = *album.Format
	}
	if album.Year != nil {
		result.Year = *album.Year
	}
	return result
}

func toAlbums(items []api.LibraryAlbum) []albums.Album {
	result := make([]albums.Album, len(items))
	for i, item := range items {
		result[i] = toAlbum(item)
	}
	return result
}
