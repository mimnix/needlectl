# needlectl

`needlectl` is a command-line client for a self-hosted [DroppedNeedle](https://www.droppedneedle.com/)
instance. It talks to DroppedNeedle over its REST API (`/api/v1`) and manages the
native library and playlists: importing CSV playlists (such as a Spotify library
export), relinking a playlist's unplayable tracks to library files, and deleting
albums or single tracks.

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
  (refusing ambiguous matches), cascades to all of its tracks, and removes the
  files from disk unless `--keep-files` is given.
- **Track deletion** — `needlectl track delete` resolves a single track by name
  (refusing ambiguous matches; same-named tracks on one album are shown with
  their disc/track number and picked with `--track`/`--disc`) and removes it.
  Note that the DroppedNeedle API always deletes the track's file from disk for
  this operation.
- **Album rename** — `needlectl album rename` renames an album through
  DroppedNeedle's staged tag-edit flow (stage a preview, verify it, then apply).
  The album field is album-scoped, so every track of the album is updated, and
  the rename is written into the audio files by the library management pipeline.
- **Track rename** — `needlectl track rename` does the same for a single track's
  title, which is track-scoped and leaves the rest of the album untouched.
- **Playlist relinking** — `needlectl playlist relink` scans an existing playlist
  and links the entries that have no source yet to matching library files. Entries
  that already play are left untouched, so a working link is never replaced.
- **Track upload with MusicBrainz matching** — `needlectl import tracks` uploads
  audio files to DroppedNeedle's drop import and then matches the entries the
  server could not identify itself to a MusicBrainz release or release group, so a
  release URL is enough instead of pasting ids into the web UI.
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
git clone <repository-url> needlectl
cd needlectl
mise install      # installs the Go toolchain pinned in mise.toml
mise run build    # produces ./bin/needlectl
```

Optionally install it onto your `PATH`:

```bash
mise run install  # go install .
```

## Quick start

```bash
# 1. Store the connection settings (prompts for server, username, password).
./bin/needlectl login

# 2. Preview what an import would do (no changes are made).
./bin/needlectl import "My Spotify Library.csv" --dry-run

# 3. Import for real.
./bin/needlectl import "My Spotify Library.csv" --name Anime
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
| `--server-url string` | Server URL; skips the prompt. |
| `--username string` | Username; skips the prompt. |
| `--skip-verify` | Save the settings without checking them against the server. |

If there is no interactive terminal (for example in CI), provide
`--server-url`/`--username` and set `NEEDLECTL_PASSWORD`.

### `needlectl import playlist <playlist.csv>`

Imports the CSV into a DroppedNeedle playlist. The playlist is found by name and
created if it does not exist; the name defaults to the CSV file name without its
extension.

`needlectl import <playlist.csv>` (without the `playlist` word) is kept as an
alias for this command.

```
$ needlectl import "My Spotify Library.csv" --name Anime
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

### `needlectl import tracks <path>...`

Uploads audio tracks to DroppedNeedle's drop import and waits for identification.
The server always identifies the files itself first; with `--mb` the entries it
could not identify (`needs_review`) are then matched to the given release group.
Entries the server imported on its own are left untouched.

The MusicBrainz reference accepts a **release URL**, a **release-group URL** or a
bare **MBID**. A release URL is resolved to its release group with a single
lookup on musicbrainz.org; a release-group reference needs no lookup at all.

```
$ needlectl import tracks ~/Downloads/"DOUBLE-DEAL (Live) - T.M.Revolution.mp3" \
    --mb https://musicbrainz.org/release/0c641ca7-3d67-4993-81b4-e2d0c2433010
› Resolving https://musicbrainz.org/release/0c641ca7-… in MusicBrainz
  release group e49c0013-dc74-4fad-b100-ae4d0fe7a1ea
› Uploading 1 file to https://music.example.com
✓ Uploaded as job 461dafd3 from DOUBLE-DEAL (Live) - T.M.Revolution.mp3
› Waiting for DroppedNeedle to identify the files
  item 15  needs_review  Loose tracks (0/1 files)
         Couldn't work out which album this is. Match it manually.
› Matching to release group e49c0013-dc74-4fad-b100-ae4d0fe7a1ea
✓ item 15 matched to T.M.R. LIVE REVOLUTION'15 -Strikes Back XI- · T.M.Revolution (1/1 files)

✓ Matched 1 entry
```

Directories are walked recursively; hidden files and directories are skipped.

Flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--mb string` | – | MusicBrainz release or release-group URL/MBID to match unidentified entries against. |
| `--wait` | `true` | Wait for identification to finish before matching. |
| `--timeout duration` | `5m` | How long to wait for identification. |
| `--dry-run` | `false` | Show what would be uploaded and matched without doing it. |

### `needlectl import match <item-id>... | --all`

Matches entries that are waiting for review, exactly like the manual match in the
web UI. Find the ids with `needlectl import jobs --status needs_review`.

```
$ needlectl import match 15 --mb https://musicbrainz.org/release-group/e49c0013-dc74-4fad-b100-ae4d0fe7a1ea
$ needlectl import match --all --mb https://musicbrainz.org/release/0c641ca7-3d67-4993-81b4-e2d0c2433010
```

Only entries that still await review are touched. Entries DroppedNeedle already
imported are reported as left alone, never overwritten.

Flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--mb string` | – | **Required.** MusicBrainz release or release-group URL/MBID. |
| `--all` | `false` | Match every entry that is waiting for review. |
| `--dry-run` | `false` | Show what would be matched without doing it. |

### `needlectl import jobs`

Lists the drop import jobs of the signed-in user together with their entries.

```
$ needlectl import jobs --status needs_review

  job:          461dafd3
  uploaded:     DOUBLE-DEAL (Live) - T.M.Revolution.mp3
  item 15  needs_review  Loose tracks (0/1 files)
         Couldn't work out which album this is. Match it manually.

✓ 1 job, 1 entry
```

Flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--all` | `false` | List the jobs of every user instead of only yours. |
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

! this permanently deletes the album, its 12 tracks and the files on disk
Delete Elza — 神崎エルザ? [y/N]: y
› Deleting album Elza — 神崎エルザ

✓ Album deleted
  album:       Elza — 神崎エルザ
  tracks:      12
  files:       delete from disk
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

> **Warning:** by default the audio files are deleted from disk. Pass
> `--keep-files` to remove only the catalog entry, and `--dry-run` to preview.

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

! this permanently deletes the track and its file from disk; DroppedNeedle has no catalog-only option for a single track
Delete Rising Hope — LiSA? [y/N]: y
› Deleting track Rising Hope — LiSA

✓ Track deleted
  track:       Rising Hope — LiSA
  removed:     1
  file:        deleted from disk
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
| `-y, --yes` | `false` | Do not ask for confirmation (required in non-interactive sessions). |
| `--dry-run` | `false` | Show what would be deleted without deleting anything. |

> **Warning:** the DroppedNeedle track-removal endpoint has **no catalog-only
> variant**, so deleting a track always deletes its file from disk. If you only
> want to drop catalog entries, use `album delete --keep-files` (album scope)
> instead.

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
`cover_url` to `/api/v1/covers/release-group/<album id>?size=250` whenever the
matched library track reports `cover_available`, so imported and relinked tracks
get artwork. Albums with no artwork in DroppedNeedle stay without one.

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
3. **Load the library** (`GET /api/v1/library/tracks`, paginated) and build a
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

Endpoints used, all under `/api/v1`:

| Method & path | Purpose |
|---------------|---------|
| `POST /auth/login` | `{username, password}` → `{token, user}` |
| `GET /auth/me` | Verify the session |
| `GET /library/tracks?q=&limit=&offset=` | Native library listing / track search (max 200 per page) |
| `GET /playlists` | List playlists |
| `POST /playlists` | `{name}` → create a playlist |
| `GET /playlists/{id}` | Playlist with tracks |
| `POST /playlists/{id}/tracks` | `{tracks, position?}` → add entries |
| `POST /playlists/{id}/tracks/remove` | `{track_ids}` → remove entries |
| `POST /playlists/{id}/resolve-sources` | Link/heal track sources (used by `playlist relink --resolve`) |
| `PATCH /playlists/{id}/tracks/{id}` | Update `source_type`/`available_sources` only (cannot set a library file) |
| `GET /library/albums?q=&page=&page_size=` | Search the library albums |
| `GET /library/albums/{id}` | Album detail (used by `album delete --id`) |
| `DELETE /library/album/{id}?delete_files=&stop_wanted=` | Remove an album and cascade its tracks |
| `DELETE /library/tracks/{id}` | Remove a single track (always deletes its file) |
| `GET /library/albums/{id}/tracks` | Album track list (used to open the tag editor) |
| `GET /library/management/tracks/{id}/tag-editor` | Editable metadata fields, scopes and revisions |
| `POST /library/management/tag-edit-previews` | Stage a metadata edit (`local_track_id`, `mode`, `fields`, `expected_settings_revision`, `expected_policy_revision`, `idempotency_key`) |
| `GET /library/management/previews/{id}` | Staged preview state (`ready_for_confirmation`, `operation_row_revision`) |
| `POST /library/management/previews/{id}/apply` | Apply a preview (`preview_token`, `expected_operation_row_revision`, `idempotency_key`, `confirmation`) |
| `POST /library/management/previews/{id}/discard` | Abandon a staged preview without applying it |
| `POST /import/uploads` | Multipart `files` → drop import job (the server identifies the files itself) |
| `GET /import/jobs?all=` | Import jobs with their entries |
| `GET /import/jobs/{id}` | One import job |
| `POST /import/items/{id}/match` | `{release_group_mbid}` → bind an entry to a release group |

Authentication uses `Authorization: Bearer <token>`. An entry is considered
*linked* when it has a `library_file_id`, a `track_source_id`, or a non-empty
`source_type`; candidate `available_sources` alone do not count.

> `DELETE /library/album/{id}` is admin-only. `delete_files` defaults to `false`
> on the API, but `needlectl` defaults to deleting files and requires
> `--keep-files` to opt out.
>
> `DELETE /library/tracks/{id}` requires the curator role (admin satisfies it)
> and accepts no options: the track's file is always deleted from disk.
>
> `PATCH /playlists/{id}/tracks/{id}` cannot attach a library file to an entry:
> it only stores `source_type`/`available_sources` and resolves the file from the
> album metadata already on the entry. Linking an existing entry therefore means
> replacing it (add at the same position, then remove the old one).
>
> `cover_url` on a playlist entry is **client-supplied and stored verbatim**:
> neither `POST /playlists/{id}/tracks` nor `resolve-sources` derives artwork from
> the linked library file. The playlist cover list is built from those same
> per-track values (`SELECT DISTINCT cover_url … LIMIT 4`), so entries added
> without a cover contribute neither per-track art nor a playlist image.
> `/api/v1/covers/release-group/{id}` takes the album's MusicBrainz release-group
> id, which is the `album_id` of a library track.
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
> way to attach a MusicBrainz reference at upload time, so the server always
> identifies the files first and matching is a second call per entry.
> `POST /import/items/{id}/match` takes a `release_group_mbid` (not a release
> id), which is why a release URL has to be resolved before matching.

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
main.go                      entry point (calls cmd.Execute)
cmd/
  root.go                    root command, global flags, config loading
  login.go                   interactive login
  import.go                  import command group (legacy CSV alias + subcommands)
  import_playlist.go         CSV playlist import
  import_tracks.go           upload tracks and match them
  import_match.go            match import entries to a MusicBrainz release
  import_jobs.go             list import jobs and their entries
  import_mb.go               MusicBrainz reference resolution for --mb
  album.go                   album delete + album rename commands
  track.go                   track delete + track rename commands
  tag_edit.go                shared staged tag-edit flow (preview, confirm, apply)
  playlist.go                playlist relink command and summary
internal/
  config/                    ~/.needlectl.conf load/save/validation
  api/                       typed DroppedNeedle REST client
  playlist/                  CSV parser
  matcher/                   normalization + library matching
  link/                      shared library-linking primitives
  albums/                    album query resolution (unique-match only)
  tracks/                    track query resolution (unique-match only)
  relink/                    playlist relink plan + orchestration
  importer/                  pure import plan + orchestration
  mbref/                     MusicBrainz reference parsing + release→group lookup
  output/                    coloured, structured terminal output
```

The decision logic is pure and is the main unit-test target:
`internal/importer/plan.go` (what an import changes) and
`internal/relink/plan.go` (which playlist entries can be linked). The API client
is tested with `httptest` and each runner with an in-memory fake API.

## Troubleshooting

| Symptom | Cause / fix |
|---------|-------------|
| `config file not found: …` | Run `needlectl login` to create `~/.needlectl.conf`. |
| `config file is incomplete: missing password` | Re-run `needlectl login`, or set `NEEDLECTL_PASSWORD`. |
| `… login failed: Invalid username or password` | Check the credentials stored in the config. |
| Login rejected with an unsupported-scheme or embedded-credentials error | Use `http(s)://host[:port]` and put credentials in the config, not the URL. |
| Certificate error against a self-signed instance | Pass `--insecure` (only if you trust the network path). |
| Most tracks show as `unlinked` | The tracks are not in the DroppedNeedle library yet. Scan the library, then re-run the import — it will relink them. |
| `library listing was truncated` / offset mismatch | The server returned inconsistent pagination; the import refuses to run against a partial library. Re-run once the instance is healthy. |
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
| `refusing to delete without confirmation; re-run with --yes` | The album delete ran without a terminal. Pass `--yes` to confirm non-interactively. |
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
- `album delete` requires an admin account and is irreversible: by default it
  deletes the files from disk. It refuses to act when an album name is
  ambiguous, so it never deletes the wrong album.
- `track delete` requires the curator role and always deletes the file from
  disk: the API exposes no catalog-only variant for a single track. It refuses
  to act when a track name is ambiguous, and lists the candidates with their
  album, disc/track number, format and duration so a same-named track on the same
  album can be selected with `--track`/`--disc` instead of guessed at.
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
- `import tracks` and `import match` only fix entries that are still
  `needs_review`. Entries the server imported on its own are deliberately left
  alone: the API offers no documented way to re-match an entry that was already
  imported, and re-matching one has not been verified, so a wrong automatic match
  still has to be corrected in the web UI or by deleting and re-uploading.
- The password is stored in plain text in a `0600` file; an OS keyring backend
  would remove that class of risk.
- The commands implemented so far are `login`, `import` (`playlist`, `files`,
  `match`, `jobs`), `album delete`, `album rename`, `track delete`,
  `track rename` and `playlist relink`.

## License

Not yet specified.
