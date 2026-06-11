#!/usr/bin/env bash
# Assemble the root filesystem image for the bare-metal GPU node. Run on a build
# host with the offline package set already staged under rootfs/. Produces a
# deterministic squashfs that the PXE flow boots into.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
rootfs="${here}/rootfs"
out="${here}/boot/rootfs.squashfs"

if [[ ! -d "${rootfs}" ]]; then
  echo "error: ${rootfs} not found; stage the base root filesystem first" >&2
  exit 1
fi

mkdir -p "${here}/boot"

# Bake the offline package set into the root filesystem.
if [[ -f "${here}/squashfs/packages.list" ]]; then
  echo "installing offline packages into rootfs"
  while read -r pkg; do
    [[ -z "${pkg}" || "${pkg}" == \#* ]] && continue
    install -D "${here}/squashfs/packages/${pkg}" "${rootfs}/opt/outpost/packages/${pkg}"
  done < "${here}/squashfs/packages.list"
fi

# Deterministic squashfs: pin ownership and timestamps so repeated builds of the
# same inputs produce a byte-identical image.
mksquashfs "${rootfs}" "${out}" \
  -noappend -all-root -mkfs-time 0 -all-time 0 -no-fragments -comp zstd

echo "wrote ${out}"
