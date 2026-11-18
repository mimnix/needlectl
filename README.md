# needlectl

<p align="center">
  <img src="assets/logo.png" alt="needlectl logo">
</p>

`needlectl` is a command-line client for
[DroppedNeedle](https://www.droppedneedle.com/). It talks to DroppedNeedle over
its REST API (`/api/v3`) and manages the native library and playlists: importing
CSV playlists (such as a Spotify library export), relinking a playlist's
unplayable tracks to library files, and deleting albums or single tracks.

The import is **idempotent** and **library-aware**: tracks that already exist in
your DroppedNeedle library are linked to their library file, and entries that
were imported before the track was available are relinked once it appears.

## Features

- **CSV playlist import** with a tolerant parser (UTF-8/UTF-16 BOM, CRLF, quoted
  commas, case-insensitive and order-independent columns).
- **Library-aware linking** — each CSV row is matched against the DroppedNeedle
  native library (Unicode-normalized, exact then fuzzy title/artist/album).
- **Idempotent** — re-running an import never duplicates tracks. Duplicate rows
  in the CSV are preserved.
- **Automatic refresh** — previously unlinked entries whose track has since been
  added to the library are relinked in place.
- **Source resolution** — after importing, `resolve-sources` is called so the
  server links tracks and records every available source.
- **Album deletion** — `needlectl album delete` resolves an album by name
  (refusing ambiguous matches), cascades to all of its tracks, and moves the
  files to DroppedNeedle's recycle bin unless `--keep-files` is given.
- **Track deletion** — `needlectl track delete` resolves a single track by name
  (refusing ambiguous matches; same-named tracks on one album are shown with
  their disc/track number and picked with `--track`/`--disc`) and removes it,
  with the same `--keep-files` choice as album deletion.
- **Album rename** — `needlectl album rename` renames an album through
  DroppedNeedle's staged tag-edit flow (stage a preview, verify it, then apply).
  The album field is album-scoped, so every track of the album is updated, and
  the rename is written into the audio files by the library management pipeline.
- **Track rename** — `needlectl track rename` does the same for a single track's
  title, which is track-scoped and leaves the rest of the album untouched.
- **Playlist relinking** — `needlectl playlist relink` scans an existing playlist
  and links the entries that have no source yet to matching library files. Entries
  that already play are left untouched, so a working link is never replaced.
- **Cover repair** — `needlectl playlist repair-covers` re-links the artwork of entries
  whose stored cover URL is missing or points at the local album id instead of the
  album's MusicBrainz release group.
- **Missing-track acquisition** — `needlectl playlist request` resolves the
  albums behind a playlist's unplayable tracks through DroppedNeedle's search and
  sends one request per recording by default (reading the album's MusicBrainz
  tracklist), or one album request with `--albums`. Anything that is not an exact
  title and artist match is reported, never guessed.
- **Track upload** — `needlectl import upload` uploads audio files to
  DroppedNeedle's drop import and reports how the server identified each group;
  entries it could not identify are listed as `needs_review`.
- **Held-entry resolution** — `needlectl held list` shows the downloads and
  uploads waiting for a decision (the "unmatched" files), and `needlectl held
  match` imports one as the MusicBrainz album you name; a release URL or MBID is
  resolved to its release group.
- **`login` command** that stores credentials in `~/.needlectl.conf` (written
  `0600`).
- **Clean, informative output** with coloured progress and a summary; `--dry-run`
  previews changes; `--verbose` lists every decision.
- **No secrets on the command line** — the password is never passed as a flag.

## Requirements

- [mise](https://mise.jdx.dev/) (recommended) — manages the Go toolchain.
- A running DroppedNeedle instance reachable over HTTP(S).
- A DroppedNeedle account with permission to create playlists.

The Go version is pinned in `mise.toml` (`go = "1.27"`). If you prefer not to use
mise, install Go 1.27+ yourself and run the `go` commands directly.

## Installation

```bash
git clone https://github.com/mimnix/needlectl.git
cd needlectl
mise install      # installs the Go toolchain pinned in mise.toml
mise run build    # produces ./bin/needlectl
```

Optionally install it onto your `PATH`:

```bash
mise run install  # go install .
```

Or install a released version straight from GitHub:

```bash
go install github.com/mimnix/needlectl@latest
```

## Releases

Releases are cut from `vX.Y.Z` tags; pushing a tag runs
[`.github/workflows/release.yml`](.github/workflows/release.yml), which:

1. rejects tags that are not exactly `vX.Y.Z` (no leading zeros, no suffix) or
   that do not point at a commit on `main`;
2. runs `go vet`, a `gofmt` check and the test suite;
3. builds `linux`, `darwin` and `windows` archives for `amd64` and `arm64`,
   with the tag baked into `needlectl --version`;
4. publishes a GitHub release whose body is
   [`docs/releases/<tag>.md`](docs/releases/), with the archives, `SHA256SUMS`,
   the release notes and `CHANGELOG.md` attached.

Tagging a release:

```bash
# 1. move the Unreleased entries in CHANGELOG.md into ## [X.Y.Z]
# 2. write docs/releases/vX.Y.Z.md from docs/releases/TEMPLATE.md
git add -A && git commit -m "chore: release vX.Y.Z"
git push origin main
git tag -a vX.Y.Z -m "needlectl vX.Y.Z"
git push origin vX.Y.Z
```

The full checklist and the validation rules are in
[`docs/releases/README.md`](docs/releases/README.md); the complete history is in
[`CHANGELOG.md`](CHANGELOG.md).

## Quick start

```bash
# 1. Store the connection settings (prompts for server, username, password).
./bin/needlectl login

# 2. Preview what the playlist import would do (no changes are made).
./bin/needlectl playlist import "My Spotify Library.csv" --dry-run

# 3. Import for real.
./bin/needlectl playlist import "My Spotify Library.csv" --name Anime
```

## Usage

### `needlectl login`

Prompts for the server URL, username and password, verifies the credentials
against the instance, and writes `~/.needlectl.conf` with `0600` permissions.

```
$ needlectl login
Server URL [http://localhost:8688]: https://music.example.com
Username: admin
Password:
✓ Signed in as Admin (admin)
✓ Saved configuration to /home/you/.needlectl.conf
```

Flags:

| Flag | Description |
|------|-------------|
| `--server string` | Server URL; skips the prompt (global flag). |
| `--username string` | Username; skips the prompt. |
| `--skip-verify` | Save the settings without checking them against the server. |

If there is no interactive terminal (for example in CI), provide
`--server`/`--username` and set `NEEDLECTL_PASSWORD`.

### `needlectl playlist import <playlist.csv>`

Imports the CSV into a DroppedNeedle playlist. The playlist is found by name and
created if it does not exist; the name defaults to the CSV file name without its
extension.

```
$ needlectl playlist import "My Spotify Library.csv" --name Anime
› Reading My Spotify Library.csv
  73 tracks, 0 warning(s)
› Authenticating with https://music.example.com
✓ Signed in as Admin (admin)
› Loading DroppedNeedle library
  1204 tracks indexed
› Resolving playlist "Anime"
✓ Created playlist "Anime"
› Planning import
  73 rows · 68 matched in library · 5 missing
  to add 73 · to refresh 0 · unchanged 0
› Adding 73 tracks (68 linked, 5 unlinked)
✓ Added 73 tracks
› Resolving track sources

✓ Import complete
  playlist:    Anime
  id:          4f1c…
  library:     1204 tracks
  added:       73 (68 linked)
  refreshed:   0
  unchanged:   0
  playable:    68
  unlinked:    5
```

Flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--name string` | CSV basename | Target playlist name. |
| `--dry-run` | `false` | Show what would change without modifying anything. |
| `--resolve` | `true` | Resolve sources after importing (links tracks now available). |
| `--batch-size int` | `100` | Number of tracks sent per add request. |

Global flags (available on every command):

| Flag | Description |
|------|-------------|
| `--config string` | Config file to use (default `~/.needlectl.conf`). |
| `--server string` | Override the server URL from the config. |
| `--insecure` | Skip TLS certificate verification. |
| `-v, --verbose` | Print every decision while running. |
| `--no-color` | Disable coloured output (`NO_COLOR` is also honoured). |
| `--version` | Print the version. |

### `needlectl import upload <path>...`

Uploads audio files to DroppedNeedle's drop import and waits for identification.
DroppedNeedle always identifies the files itself: it groups them into
album-shaped items and reports how each one ended up.

```
$ needlectl import upload ~/Downloads/"DOUBLE-DEAL (Live) - T.M.Revolution.mp3"
› Uploading 1 file to https://music.example.com
✓ Uploaded as job 461dafd3 from DOUBLE-DEAL (Live) - T.M.Revolution.mp3
› Waiting for DroppedNeedle to identify the files
  item 15  imported  T.M.R. LIVE REVOLUTION'15 -Strikes Back XI- · T.M.Revolution (1/1 files)
```

An item the server could not identify is reported as `needs_review`.
**DroppedNeedle v3 removed the API for resolving those entries**, so they have to
be decided in its web UI; the command prints a warning naming them.

Directories are walked recursively; hidden files and directories are skipped.

Flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--wait` | `true` | Wait for identification to finish before reporting. |
| `--timeout duration` | `5m` | How long to wait for identification. |
| `--dry-run` | `false` | Show what would be uploaded without doing it. |

### `needlectl import list`

Lists the drop import batches of the signed-in user together with their entries.
DroppedNeedle v3 has no single-job endpoint, so the list is read from
`GET /api/v3/import/jobs`, which already carries every item.

```
$ needlectl import list --status needs_review

  job:          461dafd3
  uploaded:     DOUBLE-DEAL (Live) - T.M.Revolution.mp3
  item 15  needs_review  Loose tracks (0/1 files)
         Couldn't work out which album this is. Decide it in the web UI.

✓ 1 job, 1 entry
```

Flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--all` | `false` | List the batches of every user instead of only yours. |
| `--status string` | – | Only show entries with this status (for example `needs_review`). |

### `needlectl album delete <album>`

Deletes an album from the library and **cascades to all of its tracks**. This is
a destructive, admin-only operation, so the album is always displayed and must be
confirmed interactively unless `--yes` is passed.

```
$ needlectl album delete "Elza" --artist "神崎エルザ"
› Authenticating with https://music.example.com
✓ Signed in as Admin (admin)
› Looking up album "Elza" in the library

  album:       Elza — 神崎エルザ
  id:          9c2b…
  tracks:      12
  details:     FLAC · 2018

! this removes the album and its 12 tracks, and moves their files to the recycle bin
Delete Elza — 神崎エルザ? [y/N]: y
› Deleting album Elza — 神崎エルザ

✓ Album deleted
  album:       Elza — 神崎エルザ
  tracks:      12
  files:       move to the recycle bin
  wanted:      stop future downloads
```

The album is looked up by name and must match **exactly one** library album. If
several albums match, the candidates are listed and nothing is deleted — narrow
the search with `--artist`, or pass the exact id with `--id`. Use `--dry-run` to
see what would happen without deleting anything.

Flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--id` | `false` | Treat the argument as an exact album id and skip the lookup. |
| `--artist string` | | Restrict the lookup to this artist. |
| `--keep-files` | `false` | Only remove the album from the catalog; keep the files on disk. |
| `--keep-wanted` | `false` | Keep the album queued for future automatic downloads. |
| `-y, --yes` | `false` | Do not ask for confirmation (required in non-interactive sessions). |
| `--dry-run` | `false` | Show what would be deleted without deleting anything. |

> **Warning:** by default the audio files are moved to DroppedNeedle's recycle
> bin. Pass `--keep-files` to remove only the catalog entry, and `--dry-run` to
> preview.

### `needlectl album rename <album> <new-name>`

Renames an album in the library. The album field is **album-scoped**, so this
renames it for every track that belongs to the album — there is no per-track
variant of this operation.

DroppedNeedle stages metadata edits rather than applying them directly, and
`needlectl` follows that flow: it reads the tag editor, stages a *tag edit
preview*, checks that the preview is ready, and only then applies it. Applying
writes the new name through the library management pipeline, which rewrites the
album's audio file tags in the background.

```
$ needlectl album rename "Elza" "Elza (Deluxe)" --artist "神崎エルザ"
› Authenticating with https://music.example.com
✓ Signed in as Admin (admin)
› Looking up album "Elza" in the library
› Reading the tag editor for Elza — 神崎エルザ
› Staging the rename "Elza" → "Elza (Deluxe)" (save-override)

  album:       Elza — 神崎エルザ
  id:          9c2b…
  from:        Elza
  to:          Elza (Deluxe)
  mode:        save-override
  preview:     4f8c…

! applying this writes the new name into the tags of all 12 tracks of this album
Rename "Elza" to "Elza (Deluxe)"? [y/N]: y

✓ Rename queued
  album:       Elza
  from:        Elza
  to:          Elza (Deluxe)
  mode:        save-override
  operation:   op_01H…
  the library management pipeline applies this in the background
```

Flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--id` | `false` | Treat the first argument as an exact album id and skip the lookup. |
| `--artist string` | | Restrict the album lookup to this artist. |
| `--mode string` | `save-override` | `save-override` stores a local override that future management runs keep; `write-once` writes the value once and a later authoritative run may replace it. |
| `-y, --yes` | `false` | Do not ask for confirmation (required in non-interactive sessions). |
| `--dry-run` | `false` | Stage and validate the preview, then discard it without applying. |

> **This writes to your audio files.** The rename is not a database-only change:
> it is published through DroppedNeedle's file organization, which rewrites the
> tags of the album's tracks. That is why the change is always confirmed and why
> `--dry-run` stages a real preview before discarding it.

If the new name equals the current one the command is a no-op. If the album's
identity has not been accepted yet, the command refuses **before staging anything**
and names the album's identity state, identification status and review — those
have to be resolved in DroppedNeedle first. If the staged preview comes back
stale, expired or not ready, nothing is applied and the reason is reported.

### `needlectl track delete <track>`

Deletes a single track from the library. The track is looked up by name and must
match **exactly one** library track; use `--artist` and/or `--album` to narrow
the search, `--track`/`--disc` to pick between tracks that share a name on one
album, or `--id` for an exact track id.

```
$ needlectl track delete "Rising Hope" --artist "LiSA"
› Authenticating with https://music.example.com
✓ Signed in as Admin (admin)
› Looking up track "Rising Hope" in the library

  track:       Rising Hope — LiSA
  id:          5a1e…
  album:       Launcher
  position:    track 1
  details:     MP3 · 4:20

! this removes the track and moves its file to the recycle bin
Delete Rising Hope — LiSA? [y/N]: y
› Deleting track Rising Hope — LiSA

✓ Track deleted
  track:       Rising Hope — LiSA
  removed:     1
  file:        move to the recycle bin
```

### Tracks that share a name on the same album

`--artist` and `--album` cannot tell two same-named tracks on one album apart, so
when the matches all sit on the same album the command refuses and lists them
with their disc/track number, format and duration. Nothing is deleted until you
point at one:

```
$ needlectl track delete "Intro" --album "Launcher"
› Looking up track "Intro" in the library
! "Intro" matches 2 tracks:
  entry-1  Intro — LiSA (Launcher) · track 3 · FLAC · 3:35
  entry-2  Intro — LiSA (Launcher) · track 7 · FLAC · 3:41
needlectl: refusing to guess: these matches share the album — re-run with --track (and --disc if the number repeats across discs), or pass --id

$ needlectl track delete "Intro" --album "Launcher" --track 7
› Looking up track "Intro" in the library

  track:       Intro — LiSA
  id:          entry-2
  album:       Launcher
  position:    track 7
  details:     FLAC · 3:41
```

For a multi-disc album, add `--disc` as well. If two rows share a name *and* a
disc/track number they are indistinguishable, and only `--id` can select one.

Flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--id` | `false` | Treat the argument as an exact track id and skip the lookup. Cannot be combined with the narrowing flags below. |
| `--artist string` | | Restrict the lookup to this artist. |
| `--album string` | | Restrict the lookup to this album. |
| `--track int` | `0` | Restrict the lookup to this track number. |
| `--disc int` | `0` | Restrict the lookup to this disc number. |
| `--keep-files` | `false` | Only remove the track from the catalog; keep the file on disk. |
| `-y, --yes` | `false` | Do not ask for confirmation (required in non-interactive sessions). |
| `--dry-run` | `false` | Show what would be deleted without deleting anything. |

> **Warning:** by default the track's file is moved to DroppedNeedle's recycle
> bin. Pass `--keep-files` to remove only the catalog entry; the file then stays
> on disk.

### `needlectl track rename <track> <new-title>`

Renames a single track in the library. Only the track's **title** changes: the
`title` field is track-scoped, so the rest of the album is untouched — use
`album rename` to rename the album itself.

The track is looked up by name and must match **exactly one** library track; use
`--artist`, `--album`, `--track` and/or `--disc` to narrow the search, or `--id`
for an exact track id.

It follows the same staged flow as `album rename`: the tag editor is read, a
preview is staged and checked, and only then is it applied. Applying writes the
new title into the track's audio file tags through the library management
pipeline, so the change is confirmed interactively unless `--yes` is given.

```
$ needlectl track rename "Disorder" "Disorder (Live)" --artist "神崎エルザ"
› Authenticating with https://music.example.com
✓ Signed in as Mimmo (admin)
› Looking up track "Disorder" in the library
› Reading the tag editor for track Disorder — 神崎エルザ
› Staging the rename "Disorder" → "Disorder (Live)" (save-override)

  track:       Disorder — 神崎エルザ
  id:          t-1
  from:        Disorder
  to:          Disorder (Live)
  mode:        save-override
  preview:     job-1

! applying this writes the new title into the tags of this track's audio file
Rename "Disorder" to "Disorder (Live)"? [y/N]: y

✓ Rename queued
  track:       Disorder — 神崎エルザ
  id:          t-1
  from:        Disorder
  to:          Disorder (Live)
  mode:        save-override
  operation:   op_01H…
  the library management pipeline applies this in the background
```

Flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--id` | `false` | Treat the first argument as an exact track id and skip the library lookup. Cannot be combined with the narrowing flags below. |
| `--artist string` | | Restrict the track lookup to this artist. |
| `--album string` | | Restrict the track lookup to this album. |
| `--track int` | `0` | Restrict the track lookup to this track number. |
| `--disc int` | `0` | Restrict the track lookup to this disc number. |
| `--mode string` | `save-override` | `save-override` stores a local override that future management runs keep; `write-once` writes the value once and a later authoritative run may replace it. |
| `-y, --yes` | `false` | Do not ask for confirmation (required in non-interactive sessions). |
| `--dry-run` | `false` | Stage and validate the preview, then discard it without applying. |

If the new title equals the current one the command is a no-op. The album's
MusicBrainz identity must already be accepted — the same gate as `album rename` —
and the refusal names the album state and review to resolve.

### `needlectl playlist relink <playlist>`

Scans the tracks of an existing playlist and links the ones that cannot be played
yet to files of the native DroppedNeedle library. This is useful when a playlist
was imported before its tracks were available, or when a library scan brought in
files that were missing earlier.

Only entries with **no source at all** (`library_file_id`, `track_source_id` and
`source_type` all empty) are considered. Entries that already play — through the
local library, Navidrome, Plex or Jellyfin — are counted but never modified.

```
$ needlectl playlist relink Anime
› Authenticating with https://music.example.com
✓ Signed in as Admin (admin)
› Loading DroppedNeedle library
  1024 tracks indexed
› Resolving playlist "Anime"
› Planning relink
  120 entries · 65 already playable · 40 to relink · 15 unmatched
  ~ Disorder — 神崎エルザ
  ~ Rising Hope — LiSA
  ~ Gurenge — LiSA
  … and 37 more (use --verbose to list them all)
› Linking 40 tracks to the library
✓ Relinked 40 tracks
› Resolving track sources

✓ Relink complete
  playlist:    Anime
  id:          pl_01H…
  library:     1024 tracks
  tracks:      120
  playable:    65
  relinked:    40
  unmatched:   15
  playable now: 105
  unlinked:    15
```

The plan lists every entry that is about to be relinked, so you can review it
before anything changes (`--dry-run` stops right after this point). The list is
capped at 20 lines; `--verbose` lists them all and appends the library file each
entry matched:

```
› Planning relink
  120 entries · 65 already playable · 40 to relink · 15 unmatched
  ~ Disorder — 神崎エルザ → 5a1e… · Elza · FLAC
  ~ Rising Hope — LiSA → 9c4b… · Launcher · MP3
  …
```

**How an entry is matched and linked:** candidates are chosen by
**title/artist/album** (Unicode-normalized: exact title+artist+album, then exact
title+artist, then a fuzzy pass), but the link itself is always written with the
**library file id** of the match (`track_source_id` = that file id, with
`source_type: local`). Titles are never used to write the link, which is why the
`--verbose` output shows the resolved file id — a fuzzy title match is visible
there instead of being silent.

Linked entries also carry the album's **cover art**. DroppedNeedle stores the
artwork reference on the playlist row itself and never derives one from the
library file, so an entry created without it shows no cover — and the playlist's
own cover is assembled from those same per-track references. `needlectl` sets
`cover_url` to `/api/v3/covers/release-group/<release group mbid>?size=250` from
the matched library track's `release_group_mbid`, so imported and relinked tracks
get artwork. `cover_available` is deliberately not a gate: it reports locally
scanned art, while the covers route also fetches art for the release group.
Entries with a missing or wrong URL can be fixed with `playlist repair-covers`.

Flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--id` | `false` | Treat the argument as an exact playlist id instead of a name. |
| `--resolve` | `true` | Also ask the server to resolve sources after relinking. |
| `--dry-run` | `false` | Show what would be relinked without modifying the playlist. |

> **Why entries are replaced, not patched:** the API's track update endpoint only
> accepts `source_type` and `available_sources` and resolves a file from the
> album metadata already stored on the entry. Spotify-imported entries usually
> have no MusicBrainz album id, so they cannot be patched. `needlectl` therefore
> inserts the linked entry at the same position and removes the old entry right
> after (insert first, so a failure can never leave a hole), which preserves the
> playlist order.

> `--resolve` complements the local matching: `resolve-sources` groups entries by
> MusicBrainz album id and links them to local/Navidrome/Plex/Jellyfin sources,
> catching tracks whose title our matcher cannot align. It cannot help entries
> that have no album id.

### `needlectl held list`

Lists the downloads and uploads DroppedNeedle kept out of the library because it
could not accept an album for them — the "unmatched" files. Nothing is lost: the
files stay on disk, and every entry records why it was held.

```
$ needlectl held list
› Authenticating with https://music.example.com
✓ Signed in as Admin (admin)
› Loading held entries

  entry:       1
  album:       Strike — Far Out
  state:       held · weak_match
  files:       1
  guess:       6b2b962f-b2fc-40ac-b418-b1fa161b96cc
  110. Far Out - Strike.mp3 · mp3 · 3:11 · track 110
  from soulseek · release ae891a44-138f-4076-b4af-fe29af2f1a5a
  The files do not match the album closely enough to import them safely.

✓ 1 entry waiting for a decision
  link one with: needlectl held match <entry-id> <musicbrainz release or release-group URL/MBID>
```

`guess` is the release group DroppedNeedle tried, not a decision. Use the entry
id with `held match` to accept a different release.

Flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--all` | `false` | Admins: list every user's entries instead of only yours. |

### `needlectl held match <entry-id> <musicbrainz-release>`

Imports one held entry as the MusicBrainz album you name. This is the manual
decision the web UI offers on a held entry: the files are paired with the album's
tracks and go into the library, without the automatic check that refused them.

DroppedNeedle links held files by **release group**, so the reference accepts a
release URL, a release-group URL or a bare MBID. A release (or a bare MBID whose
entity is unknown) is resolved to its group with a lookup on musicbrainz.org.

```
$ needlectl held match 1 https://musicbrainz.org/release/ae891a44-138f-4076-b4af-fe29af2f1a5a
…
› Resolving https://musicbrainz.org/release/ae891a44-… in MusicBrainz
✓ Resolved to release group 6b2b962f-b2fc-40ac-b418-b1fa161b96cc

  release:     6b2b962f-b2fc-40ac-b418-b1fa161b96cc
  files:       1

! this imports the entry's 1 file into the library as release group 6b2b962f-…
Import held entry 1? [y/N]: y
› Importing held entry 1 as release group 6b2b962f-…
✓ Held entry 1 resolved
  imported:    1
```

Matching imports the files; it downloads nothing. Files the server cannot pair
with the album stay held and are reported as `still held`.

Flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--yes` | `false` | Do not ask for confirmation (required when stdin is not a terminal). |
| `--dry-run` | `false` | Resolve the reference and show what would be imported without importing it. |

### `needlectl playlist repair-covers [playlist] | --all-playlists`

Repairs the album artwork shown on a playlist's tracks.

DroppedNeedle stores a cover URL on every playlist entry and never derives one
from the linked library file. Entries added while the album had no artwork, or
linked with an older needlectl that used the local album id, keep showing no
cover even after the art arrives. This command compares each linked entry with
its album's MusicBrainz release group and stores the correct artwork URL.

The API cannot patch `cover_url` (only `source_type` and `available_sources` are
writable), so an entry that needs repair is re-added at its position and the old
one removed. Playlist order is preserved; only entries linked to the local
library are touched.

```
$ needlectl playlist repair-covers Anime --dry-run
› Authenticating with https://music.example.com
✓ Signed in as Admin (admin)
› Loading DroppedNeedle library
  1582 tracks indexed
› Checking covers of "Anime"
  66 entries · 42 to repair · 24 already right · 0 not in the library · 0 without a release group
  ~ Disorder — 神崎エルザ
  ~ Rising Hope — LiSA
  …
✓ Dry run complete — no changes were made
  playlist:    Anime
  tracks:      66
  to repair:   42
  unchanged:   24
```

Run it without `--dry-run` to apply the repairs. `--all-playlists` repairs every playlist
you own and prints one line per playlist.

Flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--id` | `false` | Treat the argument as an exact playlist id instead of a name. |
| `--all-playlists` | `false` | Repair every playlist you own. Cannot be combined with a playlist argument. |
| `--dry-run` | `false` | Show what would be repaired without modifying any playlist. |

### `needlectl playlist request <playlist>`

Requests the missing tracks of a playlist.

Playlist rows usually carry no MusicBrainz ids, so each unplayable entry is
grouped by artist and album and the album is resolved through DroppedNeedle's
MusicBrainz-backed search. Only an exact album title and artist match is used.

Two modes:

- **Per track (default)** — the album's MusicBrainz tracklist is read
  (`release-group → releases → recordings`) and the entry's title is matched to a
  recording, then one `POST /requests/tracks` is sent. This fetches only that
  recording.
- **`--albums`** — one `POST /requests/albums` is sent for the release group.
  Use it when the whole album is wanted, or when a track title does not match the
  tracklist.

Curators get each download dispatched at once; other users get requests that wait
for approval. Albums already requested are skipped in both modes.

Run `playlist relink` first: entries whose track is already in the library are
linked instead of downloaded.

```
$ needlectl playlist request "Classic Rock" --dry-run
› Authenticating with https://music.example.com
✓ Signed in as Admin (admin)
› Checking the albums of "Classic Rock"
  71 entries · 71 not playable · 33 albums · per track
→ Quadrophenia — The Who: 1/1 tracks to request
→ Black Ice — AC/DC: 15/15 tracks to request
~ The Razors Edge — AC/DC: 10/11 tracks to request · 1 unmatched
✗ Duetos — Simone & Simaria: 0/3 tracks to request · 3 unmatched · no exact album and artist match

✓ Dry run complete — nothing was requested
  playlist:     Classic Rock
  mode:         per track
  tracks:       71
  not playable: 71
  albums:       33
  to request:   46
  unmatched:    25 tracks
  covered:      46 tracks
```

With `--albums`, `to request` counts albums and the report line shows the server
status (`queued`, `awaiting_approval`, `already requested`).

Flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--id` | `false` | Treat the argument as an exact playlist id instead of a name. |
| `--albums` | `false` | Request each album once instead of one recording per missing track. |
| `--dry-run` | `false` | Resolve the tracks and show what would be requested without requesting it. |

## CSV format

The parser accepts the Spotify "library" export format and similar files. The
first row must be a header; columns are matched by name, case-insensitively and
in any order. Unknown columns are ignored.

| Column (any alias) | Required | Notes |
|--------------------|----------|-------|
| `Track name`, `Title`, `Track`, `Song`, `Name` | **yes** | |
| `Artist name`, `Artist`, `Artists` | **yes** | |
| `Album`, `Album name` | no | Used to disambiguate matches. |
| `Track number`, `Track_number`, `Track no` | no | `3/12` is accepted (the part before `/`). |
| `Disc number`, `Disc_number`, `Disc` | no | |
| `Duration (ms)`, `duration_ms`, `Duration` | no | Stored on unlinked entries. |
| `Playlist name`, `Playlist` | no | Parsed, currently unused. |
| `ISRC` | no | Parsed, currently unused. |
| `Spotify - id`, `Spotify id` | no | Parsed, currently unused. |

Rows without a track or artist are skipped with a warning; a file without a
track or artist column is rejected. UTF-8 (with or without BOM), UTF-16 LE/BE,
CRLF line endings and quoted fields containing commas are all supported.

Example:

```csv
Track name,Artist name,Album,Track number,Duration (ms)
"Disorder","神崎エルザ","Elza",1,215000
"Hello, World","Café Artist","Album, Two",2,198000
```

## How the import works

1. **Read the CSV** and normalise the rows.
2. **Authenticate** with the credentials from `~/.needlectl.conf`.
3. **Load the library** (`GET /api/v3/library/tracks`, cursor-paginated) and build a
   match index.
4. **Resolve the playlist** by name, creating it if needed.
5. **Build a plan** (a pure diff, no side effects) between the CSV rows and the
   playlist's current entries.
6. **Add** new entries. A row matched to the library is added with
   `source_type: "local"` and `track_source_id: <library file id>`, which makes
   DroppedNeedle set `library_file_id` — the real link. Unmatched rows are added
   unlinked.
7. **Refresh** existing unlinked entries that now match the library: the linked
   replacement is inserted first, then the old entry is removed (so a failure
   leaves a duplicate, never a hole).
8. **Resolve sources** (`POST /playlists/{id}/resolve-sources`) so the server
   links any remaining available sources.

### Matching

`Normalize` lowercases, applies Unicode NFC normalisation, and collapses every
non-alphanumeric rune to a single space. A row matches a library track when:

1. the normalized `title + artist + album` matches exactly, or
2. the normalized `title + artist` matches exactly, or
3. a fuzzy score is high enough — title similarity must be ≥ 0.70 and the
   weighted score (55% title, 30% artist, 15% album; 65/35 without album) must
   be ≥ 0.80.

Ties are broken deterministically by library file id, so repeated runs make the
same decisions.

### Idempotency

The plan is a multiset comparison, not a set comparison: it counts how many
copies of each `(title, artist, album)` key exist in the playlist and adds only
the deficit. Re-running an unchanged import reports `added 0` and the previous
`unchanged` count.

## Configuration file

`~/.needlectl.conf` uses a minimal `key = value` syntax; values may be quoted,
blank lines and `#`/`;` comments are ignored, and unknown keys are preserved for
forward compatibility.

```ini
# needlectl configuration file.
server = "https://music.example.com"
username = "admin"
password = "secret"
insecure_skip_verify = false
```

The file stores your password, so it is written with `0600` permissions.
`needlectl` warns if it finds a file with looser permissions.

Environment variables override the file, which is useful for CI:

| Variable | Overrides |
|----------|-----------|
| `NEEDLECTL_SERVER` | `server` |
| `NEEDLECTL_USERNAME` | `username` |
| `NEEDLECTL_PASSWORD` | `password` |
| `NEEDLECTL_INSECURE_SKIP_VERIFY` | `insecure_skip_verify` |

### Server URLs

A missing scheme defaults to `https`, except for loopback hosts (`localhost`,
`127.0.0.1`, `::1`) where `http` is assumed. Only `http` and `https` are
accepted; embedded credentials (`https://user:pass@host`) are rejected. To use
plain HTTP on a remote host you must write the scheme explicitly:

```ini
server = "http://192.168.1.20:8688"
```

## API reference

Endpoints used, all under `/api/v3`:

| Method & path | Purpose |
|---------------|---------|
| `POST /auth/login` | `{username, password, transport:"bearer"}` → `{token, user}` (tokens are only issued in bearer mode) |
| `GET /me` | Verify the session |
| `GET /library/tracks?q=&limit=&after=` | Native library listing / track search (cursor-paged, max 200 per page) |
| `GET /playlists` | List playlists |
| `POST /playlists` | `{name}` → create a playlist |
| `GET /playlists/{id}` | Playlist with tracks |
| `POST /playlists/{id}/tracks` | `{tracks, position?}` → add entries |
| `POST /playlists/{id}/tracks/remove` | `{track_ids}` → remove entries |
| `POST /playlists/{id}/resolve-sources` | Link/heal track sources (used by `playlist relink --resolve`) |
| `PATCH /playlists/{id}/tracks/{id}` | Update `source_type`/`available_sources` only (cannot set a library file) |
| `GET /library/albums?q=&limit=&after=` | Search the library albums (cursor-paged) |
| `GET /library/albums/{id}` | Album detail, including the row/listing revisions deletes need |
| `DELETE /library/albums/{id}?expected_revision=&expected_listing_revision=&delete_files=&stop_wanted=` | Remove an album and cascade its tracks |
| `DELETE /library/tracks/{id}?expected_revision=&delete_file=` | Remove a single track; `delete_file=false` keeps the file on disk |
| `GET /library/albums/{id}/tracks?limit=&offset=` | Album track list (used to open the tag editor) |
| `GET /library/management/tracks/{id}/tag-editor` | Editable metadata fields, scopes and revisions |
| `POST /library/management/tag-edit-previews` | Stage a metadata edit (`local_track_id`, `mode`, `fields`, `expected_settings_revision`, `expected_policy_revision`, `idempotency_key`) |
| `GET /library/management/previews/{id}` | Staged preview state (`ready_for_confirmation`, `operation_row_revision`) |
| `POST /library/management/previews/{id}/apply` | Apply a preview (`preview_token`, `expected_operation_row_revision`, `idempotency_key`, `confirmation`) |
| `POST /library/management/previews/{id}/discard` | Abandon a staged preview (`expected_operation_row_revision`) |
| `POST /import/uploads` | Multipart `files` → drop import job (the server identifies the files itself) |
| `GET /import/jobs?all=` | Import jobs with their entries (the client filters by id) |
| `GET /held?all=` | Entries waiting for a decision (the unmatched downloads/uploads) |
| `POST /held/{id}/match` | `{release_group_mbid}` → import an entry's held files as that album |
| `GET /search/albums?q=&limit=` | MusicBrainz-backed album search (used to resolve a missing track's album) |
| `POST /requests/albums` | `{musicbrainz_id, album?, artist?, year?}` → request one release group (`playlist request --albums`) |
| `POST /requests/tracks` | `{recording_mbid, artist_name, track_title, album_title?, release_group_mbid?, duration_seconds?}` → request one recording (default of `playlist request`) |

Authentication uses `Authorization: Bearer <token>`. An entry is considered
*linked* when it has a `library_file_id`, a `track_source_id`, or a non-empty
`source_type`; candidate `available_sources` alone do not count.

> `DELETE /library/albums/{id}` is admin-only and requires both the album's
> `row_revision` and `listing_revision`. `delete_files` defaults to `false` on
> the API, but `needlectl` defaults to deleting files and requires `--keep-files`
> to opt out.
>
> `DELETE /library/tracks/{id}` requires the curator role (admin satisfies it)
> and the track's `row_revision`. `delete_file` defaults to `true` on the API
> (the file moves to the recycle bin); `needlectl` exposes `--keep-files` to send
> `delete_file=false`. Since v3 has no single-track read endpoint, `needlectl`
> scans the library listing when it was not given the revision.
>
> `GET /playlists/{id}` already includes the playlist's tracks; v3 has no
> separate track-list endpoint.
>
> `PATCH /playlists/{id}/tracks/{id}` cannot attach a library file to an entry:
> it only stores `source_type`/`available_sources` and resolves the file from the
> album metadata already on the entry. Linking an existing entry therefore means
> replacing it (add at the same position, then remove the old one).
>
> `cover_url` on a playlist entry is **client-supplied and stored verbatim**:
> neither `POST /playlists/{id}/tracks` nor `resolve-sources` derives artwork from
> the linked library file, and `PATCH /playlists/{id}/tracks/{id}` ignores it
> (only source fields are writable). The playlist cover list is built from those
> same per-track values (`SELECT DISTINCT cover_url … LIMIT 4`), so entries added
> without a cover contribute neither per-track art nor a playlist image.
> `/api/v3/covers/release-group/{id}` takes the **MusicBrainz release-group id**
> (`release_group_mbid` of a library track), not the local `album_id`.
>
> Library management (used by `album rename` and `track rename`) must be active
> on the instance. Its
> edits are **staged, journalled and published through file organization**: a
> preview is created, then applied with its `preview_token` and the
> `operation_row_revision` it reported. Field `scope` matters — `album`,
> `album_artist` and the `*_sort` fields are album-scoped, while `title` and
> `artist` are track-scoped. These endpoints rewrite audio file tags.
>
> `POST /playlists/{id}/resolve-sources` only resolves entries that have both an
> `album_id` (MusicBrainz release-group id) and a `track_number`; entries without
> them keep their stored sources.
>
> `POST /import/uploads` accepts **only** the multipart `files` field: there is no
> way to attach a MusicBrainz reference at upload time. DroppedNeedle v3 removed
> the per-item match endpoint as well, so entries that end in `needs_review` can
> only be resolved in the server's web UI.
>
> `POST /held/{id}/match` takes a **release group**, not a release; `needlectl`
> resolves a release URL or MBID to its group on musicbrainz.org before calling
> it. The endpoint imports every file of the entry and reports how many were
> imported, were already owned, stayed held or were discarded.
>
> `POST /requests/albums` takes a **release-group MBID** and answers 202. Curators
> get the download dispatched at once; other users get `awaiting_approval`.
> Duplicate asks attach to the live row. `GET /search/albums` is the
> MusicBrainz-backed lookup `playlist request` uses to turn a playlist's album
> name into that id; `in_library` and `requested` on a hit tell whether a request
> is still needed.
>
> `POST /requests/tracks` takes a **recording MBID** and fetches that one
> recording. The server's `/search/tracks` only returns library rows, so
> `playlist request` resolves the recording from the album's MusicBrainz
> tracklist (`release-group/{rg}?inc=releases`, then
> `release/{id}?inc=recordings`) and matches the playlist title exactly.

## Development

```bash
mise tasks          # list every task
mise run build      # compile ./bin/needlectl
mise run run        # build and run (e.g. mise run run -- import file.csv)
mise run test       # unit tests
mise run test:race  # unit tests with the race detector
mise run cover      # tests + coverage.out/coverage.html
mise run lint       # go vet + gofmt check
mise run fmt        # gofmt -w .
mise run tidy       # go mod tidy
mise run install    # go install .
mise run clean      # remove build/coverage artifacts
mise run ci         # lint + test:race + build
```

### Project layout

```
.github/
  workflows/release.yml      tag-driven release: validate, test, build, publish
main.go                      entry point (calls cmd.Execute)
CHANGELOG.md                 complete technical history (Keep a Changelog)
cmd/
  root.go                    root command, global flags, config loading
  login.go                   interactive login
  import.go                  import command group (upload + list)
  import_playlist.go         CSV playlist import
  import_upload.go           upload audio files
  import_list.go             list import batches and their entries
  held.go                    held-entry list + MusicBrainz release match
  album.go                   album delete + album rename commands
  track.go                   track delete + track rename commands
  tag_edit.go                shared staged tag-edit flow (preview, confirm, apply)
  playlist.go                playlist command group and relink summary
  playlist_repair_covers.go  playlist cover repair command
  playlist_request.go        missing-track acquisition command (track or album)
internal/
  config/                    ~/.needlectl.conf load/save/validation
  api/                       typed DroppedNeedle REST client
  playlist/                  CSV parser
  matcher/                   normalization + library matching
  link/                      shared library-linking primitives
  albums/                    album query resolution (unique-match only)
  tracks/                    track query resolution (unique-match only)
  mbref/                     MusicBrainz reference, release-group and tracklist lookup
  relink/                    playlist relink plan + orchestration
  covers/                    playlist cover repair plan + orchestration
  acquire/                   missing-track album resolution + requests
  importer/                  pure import plan + orchestration
  output/                    coloured, structured terminal output
docs/
  releases/                  one Markdown file per release, used as release body
scripts/
  release/build.sh           cross-compile archives + SHA256SUMS
  release/notes.sh           compose the release body from docs + CHANGELOG
assets/
  logo.png                   project logo, shown at the top of this README
```

The decision logic is pure and is the main unit-test target:
`internal/importer/plan.go` (what an import changes),
`internal/relink/plan.go` (which playlist entries can be linked),
`internal/covers/plan.go` (which cover URLs need repair) and
`internal/acquire/plan.go` (which missing albums and recordings resolve). The API
client is tested with `httptest` and each runner with an in-memory fake API.

## Troubleshooting

| Symptom | Cause / fix |
|---------|-------------|
| `config file not found: …` | Run `needlectl login` to create `~/.needlectl.conf`. |
| `config file is incomplete: missing password` | Re-run `needlectl login`, or set `NEEDLECTL_PASSWORD`. |
| `… login failed: Invalid username or password` | Check the credentials stored in the config. |
| Login rejected with an unsupported-scheme or embedded-credentials error | Use `http(s)://host[:port]` and put credentials in the config, not the URL. |
| Certificate error against a self-signed instance | Pass `--insecure` (only if you trust the network path). |
| Most tracks show as `unlinked` | The tracks are not in the DroppedNeedle library yet. Scan the library, then re-run the import — it will relink them. |
| `library listing was truncated` / repeated page cursor | The server returned inconsistent pagination; the command refuses to run against a partial library. Re-run once the instance is healthy. |
| `refusing to guess: narrow the search with --artist or pass --id` | More than one album matched. Re-run with `--artist`, or copy the id from the listed candidates and use `--id`. |
| `refusing to guess: narrow the search with --artist/--album, or pass --id` | More than one track matched across albums. Re-run with `--artist`/`--album`, or copy the id from the listed candidates and use `--id`. |
| `refusing to guess: these matches share the album …` | Same-named tracks on one album, where `--artist`/`--album` cannot help. Re-run with `--track` (add `--disc` for multi-disc albums), or copy the id and use `--id`. |
| `--id cannot be combined with --artist, --album, --track or --disc` | `--id` skips the lookup, so the narrowing flags would be silently ignored. Pass one or the other. |
| `the staged preview is stale: …` | The album or the management settings changed between reading the tag editor and applying. Re-run `album rename`. |
| `the staged preview expired — run the command again` | Previews have a lifetime. Re-run `album rename` to stage a fresh one. |
| `the staged preview is not ready to apply (…)` | The server is still validating, or found no eligible files. Nothing was applied; check the instance's library management status. |
| `album …` / `track …` `cannot be renamed: its MusicBrainz identity has not been accepted (…)` | DroppedNeedle only allows metadata edits on albums whose identity is accepted, and a track's edits depend on its album. The message names the album state, the identification status and the review. Resolve that review first (accept the candidate, retry identification, keep-tagged, or `reidentify` the album), or tag the files with MusicBrainz ids and rescan. Nothing was staged. |
| `applying this writes the new name into the tags …` then `aborted` | You answered no to the confirmation, so the staged preview was discarded and nothing changed. |
| `unknown mode "…" (use save-override or write-once)` | Fix the `--mode` value. |
| `refusing to delete/import without confirmation; re-run with --yes` | The command ran without a terminal. Pass `--yes` to confirm non-interactively. |
| `held match` reports `no held entry …` | The entry is not among your held entries. Run `needlectl held list` and use the id it shows. |
| `playlist request` reports unmatched tracks | The album name/artist or the track title does not match MusicBrainz exactly. Fix the tags, request the album in the web UI, or pass `--albums` to request the whole release group. |
| `MusicBrainz answered 503` or repeated `429` | The client retries with backoff. If it keeps failing, wait a minute and re-run; MusicBrainz allows about one request per second. |
| `too many albums match …` | The album query was too broad to page through. Refine the query or use `--id`. |
| `playlist "…" not found` | Check the name (`needlectl playlist relink` matches case-insensitively), or pass the id with `--id`. |
| `refusing to guess: pass the playlist id with --id` | Several playlists share that name. Copy the id from the listed candidates and use `--id`. |
| `playlist relink` reports entries as `unmatched` | Those tracks are not in the DroppedNeedle library yet. Scan the library, then re-run `playlist relink`. |
| A duplicate entry appears after an interrupted run | The refresh/relink inserts before removing; re-run the command to reconcile. |

## Privacy

Playlist exports contain personal listening data. `*.csv` files are gitignored
except for the synthetic fixture in `internal/playlist/testdata/`. Keep your
exports and `~/.needlectl.conf` private.

## Known limitations

- Playlists are matched by name. `import` uses the first case-insensitive match;
  `playlist relink` prefers correctness and refuses an ambiguous name, listing the
  candidates so you can pass `--id`.
- `album delete` requires an admin account. By default it moves the album's
  files to the recycle bin; `--keep-files` removes only the catalog entry. It
  refuses to act when an album name is ambiguous, so it never deletes the wrong
  album.
- `track delete` requires the curator role. By default it moves the track's file
  to the recycle bin; `--keep-files` keeps it on disk, though a library rescan may
  add it back as a new row. It refuses to act when a track name is ambiguous, and
  lists the candidates with their album, disc/track number, format and duration
  so a same-named track on the same album can be selected with
  `--track`/`--disc` instead of guessed at.
- `playlist relink` only touches entries that have no source at all. Entries that
  play through another source (Navidrome, Plex, Jellyfin) already have a link, so
  they are never re-pointed at the local library.
- `album rename` changes the album field, which is album-scoped, so the whole
  album is renamed. `track rename` changes only the track's `title`. Both require
  library management to be active and write into the audio file tags through the
  server's file organization, so they are staged, confirmed and applied rather
  than applied immediately. Other track-scoped fields (`artist`, `genre`, …) are
  not exposed as flags yet, and the server's identity gate applies to both.
- Duration is stored but not used as a matching guard, so live/remix versions
  with very similar titles could be linked to the wrong file.
- `import upload` only reports entries that are still `needs_review`;
  DroppedNeedle v3 dropped the API to resolve them, so it points at the web UI
  instead. Entries the server imported on its own are deliberately left
  alone: the API offers no documented way to re-match an entry that was already
  imported, and re-matching one has not been verified, so a wrong automatic match
  still has to be corrected in the web UI or by deleting and re-uploading.
- The password is stored in plain text in a `0600` file; an OS keyring backend
  would remove that class of risk.
- The commands implemented so far are `login`, `playlist import`, `playlist
  relink`, `playlist repair-covers`, `playlist request`, `import upload`,
  `import list`, `held list`, `held match`, `album delete`, `album rename`,
  `track delete` and `track rename`.

## License

Released under the [MIT License](LICENSE). © 2026 mimnix.
