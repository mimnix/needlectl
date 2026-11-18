# Changelog

All notable changes to needlectl are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

This file is the complete technical history. The user-facing announcement of
each release lives in [`docs/releases/`](docs/releases/) and is what the release
workflow publishes on GitHub.

## [Unreleased]

### Added

- CSV playlist import with a tolerant parser (UTF-8/UTF-16 BOM, CRLF, quoted
  commas, case-insensitive and order-independent columns).
- Library-aware linking: every CSV row is matched against the DroppedNeedle
  native library, and re-running an import never duplicates tracks.
- Automatic refresh of previously unlinked entries once their track appears in
  the library, plus `resolve-sources` so the server records every source.
- `playlist relink` for existing playlists, which only touches entries that
  have no source yet.
- `playlist repair-covers` to re-link cover art through each album's MusicBrainz
  release group.
- `playlist request` for missing tracks: per-recording requests driven by the
  album's MusicBrainz tracklist, or one release-group request with `--albums`.
- `import upload` for the drop import and `import list` for its batches.
- `held list` and `held match` to review and resolve unmatched downloads.
- `album` and `track` `delete`/`rename`, with recycle-bin semantics and a staged
  tag-edit flow backed by DroppedNeedle's library management.
- `login` that stores the connection settings in `~/.needlectl.conf` (`0600`).
- Coloured output with `--dry-run` previews, `--verbose` decisions, and no
  secrets on the command line.
- Resource-first command grammar: every command is `<resource> <operation>`.
- MIT license.

[Unreleased]: https://github.com/mimnix/needlectl/commits/main
