#!/usr/bin/env bash
# Deploy step (air-gapped host): load the mirrored archives into the bundle's
# local registry. No network access required.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
registry="registry.outpost.local:5000"

load_image() {
  local archive="$1" local_ref="$2"
  echo "loading $(basename "${archive}") -> ${local_ref}"
  skopeo copy \
    "docker-archive:${here}/$(basename "${archive}")" \
    "docker://${local_ref}" \
    --dest-tls-verify=false
}

load_image "images/example-vllm-server.tar" "registry.outpost.local:5000/example/vllm-server:0.6.3"

echo "all images loaded into ${registry}"
