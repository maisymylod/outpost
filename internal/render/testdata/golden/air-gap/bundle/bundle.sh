#!/usr/bin/env bash
# Package the whole bundle into a single reproducible tarball. Given identical
# bundle contents, this produces a byte-identical archive (sorted entries,
# pinned ownership, fixed mtime), so the bundle can be checksummed and audited.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
name="$(basename "${here}")"
out="${1:-${here}/../${name}.tar.gz}"

# A fixed mtime keeps the archive deterministic. 2020-01-01 is arbitrary but
# stable; do not use the current time.
fixed_mtime="2020-01-01 00:00:00"

tar \
  --sort=name \
  --owner=0 --group=0 --numeric-owner \
  --mtime="${fixed_mtime}" \
  --format=gnu \
  -C "$(dirname "${here}")" \
  -cf - "${name}" \
  | gzip -n > "${out}"

echo "wrote ${out}"
sha256sum "${out}" 2>/dev/null || shasum -a 256 "${out}"
