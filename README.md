# outpost

Multi-target deployment generator: one declarative workload spec, rendered for
cloud, on-prem, and air-gapped bare-metal. Every rendered artifact is validated
by a real tool (terraform, helm, kubeconform, shellcheck), not just produced.

> Status: under construction. Phase 1 (spec + CLI) is in place; targets land
> phase by phase.

## What it does

`outpost` reads one `workload.yaml` describing a GPU service and renders
deployment artifacts for three targets:

| Target    | Output |
|-----------|--------|
| `cloud`   | Terraform (managed K8s + GPU node pool) and Helm values |
| `on-prem` | Helm chart, `Workload` CRD, and a controller-runtime operator |
| `air-gap` | Offline bundle: image manifest, PXE config, cloud-init, squashfs build |

## Quickstart

```sh
go build -o bin/outpost .
./bin/outpost render --target cloud
./bin/outpost render --target on-prem
./bin/outpost render --air-gap
```

The spec is `workload.yaml` by default (`--spec` to override). Artifacts land
under `out/<target>/` (`--out` to override).

## Design

- **Go, idiomatic, deterministic.** The same spec always renders byte-identical
  output: sorted keys, fixed file modes, no timestamps.
- **One spec, many targets.** `internal/spec` owns the YAML; each target
  implements a `Renderer` and registers itself.
- **Validated, not just generated.** Golden-file tests plus per-target real-tool
  validators run in CI.

More detail (architecture diagram, the full gap-mapping table, and a one-command
demo) lands with Phase 5.
