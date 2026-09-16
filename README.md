# needlectl

`needlectl` is a command-line client for a self-hosted [DroppedNeedle](https://www.droppedneedle.com/)
instance. It talks to DroppedNeedle over its REST API (`/api/v1`) and currently
supports importing CSV playlists (such as a Spotify library export) into
DroppedNeedle playlists.

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
  (refusing ambiguous matches) and removes it. Note that the DroppedNeedle API
  always deletes the track's file from disk for this operation.
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

### `needlectl import <playlist.csv>`

Imports the CSV into a DroppedNeedle playlist. The playlist is found by name and
created if it does not exist; the name defaults to the CSV file name without its
extension.

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

### `needlectl track delete <track>`

Deletes a single track from the library. The track is looked up by name and must
match **exactly one** library track; use `--artist` and/or `--album` to narrow
the search, or `--id` for an exact track id.

```
$ needlectl track delete "Rising Hope" --artist "LiSA"
› Authenticating with https://music.example.com
✓ Signed in as Admin (admin)
› Looking up track "Rising Hope" in the library

  track:       Rising Hope — LiSA
  id:          5a1e…
  album:       Launcher
  details:     MP3 · 4:20

! this permanently deletes the track and its file from disk; DroppedNeedle has no catalog-only option for a single track
Delete Rising Hope — LiSA? [y/N]: y
› Deleting track Rising Hope — LiSA

✓ Track deleted
  track:       Rising Hope — LiSA
  removed:     1
  file:        deleted from disk
```

If several tracks match, the candidates (id, title, artist and album) are listed
and nothing is deleted. Use `--dry-run` to see what would happen without
deleting anything.

Flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--id` | `false` | Treat the argument as an exact track id and skip the lookup. |
| `--artist string` | | Restrict the lookup to this artist. |
| `--album string` | | Restrict the lookup to this album. |
| `-y, --yes` | `false` | Do not ask for confirmation (required in non-interactive sessions). |
| `--dry-run` | `false` | Show what would be deleted without deleting anything. |

> **Warning:** the DroppedNeedle track-removal endpoint has **no catalog-only
> variant**, so deleting a track always deletes its file from disk. If you only
> want to drop catalog entries, use `album delete --keep-files` (album scope)
> instead.

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
| `POST /playlists/{id}/resolve-sources` | Link/heal track sources |
| `GET /library/albums?q=&page=&page_size=` | Search the library albums |
| `GET /library/albums/{id}` | Album detail (used by `album delete --id`) |
| `DELETE /library/album/{id}?delete_files=&stop_wanted=` | Remove an album and cascade its tracks |
| `DELETE /library/tracks/{id}` | Remove a single track (always deletes its file) |

Authentication uses `Authorization: Bearer <token>`. An entry is considered
*linked* when it has a `library_file_id`, a `track_source_id`, or a non-empty
`source_type`; candidate `available_sources` alone do not count.

> `DELETE /library/album/{id}` is admin-only. `delete_files` defaults to `false`
> on the API, but `needlectl` defaults to deleting files and requires
> `--keep-files` to opt out.
>
> `DELETE /library/tracks/{id}` requires the curator role (admin satisfies it)
> and accepts no options: the track's file is always deleted from disk.

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
  import.go                  import command and summary
  album.go                   album delete command and confirmation
  track.go                   track delete command and confirmation
internal/
  config/                    ~/.needlectl.conf load/save/validation
  api/                       typed DroppedNeedle REST client
  playlist/                  CSV parser
  matcher/                   normalization + library matching
  albums/                    album query resolution (unique-match only)
  tracks/                    track query resolution (unique-match only)
  importer/                  pure import plan + orchestration
  output/                    coloured, structured terminal output
```

The import decision logic (`internal/importer/plan.go`) is a pure function and
is the main unit-test target; the API client is tested with `httptest` and the
runner with an in-memory fake API.

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
| `refusing to guess: narrow the search with --artist/--album or pass --id` | More than one track matched. Re-run with `--artist`/`--album`, or copy the id from the listed candidates and use `--id`. |
| `refusing to delete without confirmation; re-run with --yes` | The album delete ran without a terminal. Pass `--yes` to confirm non-interactively. |
| `too many albums match …` | The album query was too broad to page through. Refine the query or use `--id`. |
| A duplicate entry appears after an interrupted run | The refresh inserts before removing; re-run the import to reconcile. |

## Privacy

Playlist exports contain personal listening data. `*.csv` files are gitignored
except for the synthetic fixture in `internal/playlist/testdata/`. Keep your
exports and `~/.needlectl.conf` private.

## Known limitations

- Playlists are matched by name; if several playlists share a name the first
  match is used (a `--playlist-id` flag could disambiguate in the future).
- `album delete` requires an admin account and is irreversible: by default it
  deletes the files from disk. It refuses to act when an album name is
  ambiguous, so it never deletes the wrong album.
- `track delete` requires the curator role and always deletes the file from
  disk: the API exposes no catalog-only variant for a single track. It refuses
  to act when a track name is ambiguous.
- Duration is stored but not used as a matching guard, so live/remix versions
  with very similar titles could be linked to the wrong file.
- The password is stored in plain text in a `0600` file; an OS keyring backend
  would remove that class of risk.
- Only the `import` and `login` commands exist so far.

## License

Not yet specified.
