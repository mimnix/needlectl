#!/usr/bin/env bash
#
# Cross-compiles needlectl for every released platform.
#
# Usage:
#   scripts/release/build.sh <tag> [output-dir]
#
# <tag> is a vX.Y.Z tag; it is baked into `needlectl --version`. The output
# directory is wiped first and ends up with one archive per target plus
# SHA256SUMS. Each archive contains the binary, LICENSE and README.md.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"

tag="${1:?usage: build.sh <tag> [output-dir]}"
out="${2:-dist}"

module="$(go list -m)"

targets=(
  linux/amd64
  linux/arm64
  darwin/amd64
  darwin/arm64
  windows/amd64
  windows/arm64
)

rm -rf "$out"
mkdir -p "$out"

for target in "${targets[@]}"; do
  os="${target%/*}"
  arch="${target#*/}"

  binary="needlectl"
  if [ "$os" = "windows" ]; then
    binary="needlectl.exe"
  fi

  pkg="needlectl_${tag}_${os}_${arch}"
  stage="$out/$pkg"
  mkdir -p "$stage"

  echo "building $pkg"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" GOFLAGS="-trimpath" \
    go build -ldflags "-s -w -X ${module}/cmd.version=${tag}" \
    -o "$stage/$binary" .

  cp LICENSE README.md "$stage/"

  if [ "$os" = "windows" ]; then
    if command -v zip >/dev/null 2>&1; then
      (cd "$out" && zip -qr "$pkg.zip" "$pkg")
    elif command -v python3 >/dev/null 2>&1; then
      python3 - "$out/$pkg.zip" "$stage" "$pkg" <<'PY'
import os, sys, zipfile

archive, root, base = sys.argv[1], sys.argv[2], sys.argv[3]
with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED) as bundle:
    for dirpath, _, names in os.walk(root):
        for name in names:
            path = os.path.join(dirpath, name)
            bundle.write(path, os.path.join(base, os.path.relpath(path, root)))
PY
    else
      echo "build.sh: need 'zip' or 'python3' to build the Windows archive" >&2
      exit 1
    fi
  else
    tar -C "$out" -czf "$out/$pkg.tar.gz" "$pkg"
  fi

  rm -rf "$stage"
done

(cd "$out" && sha256sum needlectl_* > SHA256SUMS)

echo "archives written to $out:"
ls -1 "$out"
