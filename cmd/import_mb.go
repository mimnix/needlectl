package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/mimmo/needlectl/internal/mbref"
	"github.com/mimmo/needlectl/internal/output"
)

// resolveReleaseGroup turns a MusicBrainz reference into the release-group MBID
// the import matching endpoint expects.
//
// A release-group reference needs no network access; a release URL or a bare
// MBID is looked up on musicbrainz.org.
func resolveReleaseGroup(ctx context.Context, printer *output.Printer, raw string) (string, error) {
	ref, err := mbref.Parse(raw)
	if err != nil {
		return "", err
	}
	if ref.Kind == mbref.KindReleaseGroup {
		return ref.MBID, nil
	}

	printer.Step("Resolving %s in MusicBrainz", raw)
	group, err := mbref.NewResolver().ResolveReleaseGroup(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", raw, err)
	}
	printer.Info("release group %s", group)
	return group, nil
}

// statusSuffix renders the "with status X" tail of a message, or nothing.
func statusSuffix(status string) string {
	if strings.TrimSpace(status) == "" {
		return ""
	}
	return " with status " + strings.TrimSpace(status)
}
