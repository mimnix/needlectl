# Release notes

Every release has exactly one Markdown file here, named after its tag:

```
docs/releases/vX.Y.Z.md
```

The release workflow uses that file verbatim as the body of the GitHub release
and attaches a copy plus [`CHANGELOG.md`](../../CHANGELOG.md) to it, so users
get the announcement and the full history with the archives.

The two documents have different jobs:

| File | Audience | Content |
|------|----------|---------|
| `docs/releases/vX.Y.Z.md` | users | highlights, upgrade notes, links |
| `CHANGELOG.md` | maintainers | every added/changed/fixed/removed item |

## Workflow

A release is published by pushing a tag; there is no manual upload step.

1. Move the `Unreleased` entries of `CHANGELOG.md` into a new
   `## [X.Y.Z] - YYYY-MM-DD` section.
2. Copy [`TEMPLATE.md`](TEMPLATE.md) to `docs/releases/vX.Y.Z.md` and fill it
   in.
3. Commit the two files: `git commit -am "chore: release vX.Y.Z"`.
4. Push `main`, then push the tag:

   ```bash
   git push origin main
   git tag -a vX.Y.Z -m "needlectl vX.Y.Z"
   git push origin vX.Y.Z
   ```

5. The workflow runs lint and tests, builds the archives and publishes the
   release.

## Validation

`.github/workflows/release.yml` refuses to publish when:

- the tag is not exactly `vX.Y.Z` — no leading zeros, no `-rc`/build suffix;
- `docs/releases/<tag>.md` does not exist;
- `CHANGELOG.md` has no non-empty section for the version;
- the tagged commit is not on `main`;
- a release with the same tag already exists.

## Artifacts

| Asset | Content |
|-------|---------|
| `needlectl_vX.Y.Z_<os>_<arch>.tar.gz` | Linux and macOS builds (`needlectl`, `LICENSE`, `README.md`) |
| `needlectl_vX.Y.Z_<os>_<arch>.zip` | Windows builds (same content) |
| `SHA256SUMS` | checksums of every archive |
| `RELEASE_NOTES.md` | composed release body (notes + installation + changelog) |
| `needlectl_vX.Y.Z.md` | copy of the release notes file |
| `CHANGELOG.md` | full technical history |

Supported targets: `linux`, `darwin` and `windows` on `amd64` and `arm64`.

## Local checks

The same scripts the workflow uses can be run by hand:

```bash
scripts/release/notes.sh v0.1.0                    # preview the release body
scripts/release/build.sh v0.1.0 /tmp/needlectl-dist # build every archive
```

`notes.sh` fails if the notes file or the changelog section is missing, which is
the same contract the workflow enforces.
