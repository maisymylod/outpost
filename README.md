# outpost

Multi-target deployment generator: one declarative workload spec, rendered for
cloud, on-prem, and air-gapped bare-metal. Every rendered artifact is validated
by a real tool (terraform, helm, kubeconform, shellcheck), not just produced.

The running example is a multi-GPU inference server. One `workload.yaml`
describes it once; `outpost` renders deployable artifacts for three very
different environments, with the GPU specifics (device plugin, NCCL,
InfiniBand, node affinity) wired correctly in each.

## What it does

One `Renderer` interface, three implementations. From a single spec, outpost
renders:

- **cloud**: Terraform for a managed EKS cluster with a GPU node group, plus a
  `helm_release` for the workload. Passes `terraform validate` and `fmt -check`.
- **on-prem**: a Helm chart carrying a `Workload` CRD, RBAC, a
  controller-runtime operator, and a `Workload` CR. The operator reconciles the
  CR into a GPU-wired Deployment and Service.
- **air-gap**: a self-contained offline bundle (image manifest and mirror
  scripts, PXE boot config, cloud-init, deterministic squashfs build) with no
  external registry or URL anywhere in the deploy path.

The running example is a multi-GPU inference server, so the GPU specifics are
real in every target: `nvidia.com/gpu` requests, device-plugin tolerations and
node affinity, NCCL and InfiniBand env, an RDMA device, hugepages, and shared
memory. Output is deterministic (byte-identical re-render), and every artifact
is validated by the real tool (terraform, helm, kubeconform, shellcheck) in CI
across `linux/amd64`, `linux/arm64`, and `darwin/arm64`.

## Architecture

```
                       workload.yaml  (one spec)
                              |
                      +-------v--------+
                      |  outpost CLI   |   parse + validate (internal/spec)
                      +-------+--------+
                              |  Renderer interface (internal/render)
            +-----------------+------------------+
      +-----v------+     +-----v------+     +-----v-------+
      |   cloud    |     |  on-prem   |     |   air-gap   |
      | Terraform  |     | Helm chart |     | PXE boot    |
      | EKS + GPU  |     | + CRD +    |     | cloud-init  |
      | node group |     | operator   |     | squashfs    |
      | helm_release|    | + CR       |     | image bundle|
      +-----+------+     +-----+------+     +-----+-------+
            |                  |                  |
     terraform validate    helm lint        offline-purity
     terraform fmt -check   kubeconform      shellcheck
                            (operator: reconcile test, fake client)
```

The on-prem operator is a real, separate Go module (`operator/`). It is built,
vetted, and reconcile-tested in CI. The on-prem render produces the artifacts
that deploy it (CRD, RBAC, operator Deployment) plus a `Workload` CR built from
the spec; the operator turns that CR into the GPU-wired Deployment and Service.

## Quickstart

```sh
# Render all three targets from the example spec into _demo/
make demo

# Or build and render a single target
go build -o bin/outpost .
./bin/outpost render --target cloud
./bin/outpost render --target on-prem
./bin/outpost render --air-gap
```

`--spec` overrides the input (default `workload.yaml`); `--out` overrides the
output directory (default `out/`, one subdir per target).

## The spec

```yaml
apiVersion: outpost.dev/v1
kind: Workload
metadata:
  name: llm-inference
  namespace: inference
workload:
  image: ghcr.io/example/vllm-server:0.6.3
  replicas: 2
  gpu:
    count: 4                  # GPUs per replica
    class: nvidia-a100-80gb
    interconnect:
      nccl: true
      infiniband: true
  resources:
    cpu: "16"
    memory: 128Gi
    hugepages: 2Gi
  env: { MODEL_ID: meta-llama/Llama-3.1-70B-Instruct, TENSOR_PARALLEL_SIZE: "4" }
  ports:
    - { name: http, port: 8000 }
  targets: [cloud, on-prem, air-gap]
```

## Targets

### cloud
Terraform for a managed EKS cluster with a GPU managed node group
(`AL2_x86_64_GPU` AMI, GPU taint and labels, instance type derived from the GPU
class), plus a `helm_release` that deploys the workload chart. Spec values flow
into `terraform.auto.tfvars` and the chart `values.yaml`.

### on-prem
A Helm chart that installs the `Workload` CRD, the operator (RBAC + Deployment),
and a `Workload` CR built from the spec. The operator reconciles that CR into a
Deployment and Service with the full GPU wiring: `nvidia.com/gpu` requests,
NVIDIA device-plugin tolerations and `nvidia.com/gpu.product` node affinity,
NCCL and InfiniBand env, an `rdma/ib` device, hugepages, and a `/dev/shm`
volume.

### air-gap
A self-contained bundle: an image manifest plus an online mirror script (the
source registry is supplied at runtime via `$SOURCE_REGISTRY`, so the bundle
itself names no external registry), an offline image loader, a PXE boot config,
cloud-init that configures a local containerd registry mirror and deploys the
GPU workload, a deterministic squashfs build script, and a reproducible
`bundle.sh`. Nothing in the deploy path references an external registry or URL.

## Validation

Every target is checked by the real tool, run inside the Go test suite (and so
in CI). Tests that need a tool skip cleanly when it is absent.

| Target | Validators |
|---|---|
| all | `go vet`, `gofmt`, golden-file diff, determinism (byte-identical re-render) |
| cloud | `terraform fmt -check`, `terraform init` + `terraform validate` |
| on-prem | `helm lint`, `helm template --include-crds \| kubeconform`, CRD structural test, operator reconcile test |
| air-gap | offline-purity scan (no external registry or URL), `shellcheck` |

```sh
make test       # everything, both modules
make validate   # just the per-target validators, verbose
```

kubeconform strictly validates the native Kubernetes resources; it skips the
CRD and the custom `Workload` kind because no published schema exists for them
(the standard pattern). The CRD is covered separately by a structural test, and
the custom resource by the operator's reconcile test.

## Determinism

The same spec always renders byte-identical artifacts: map keys are sorted, file
modes are fixed, and no timestamps are embedded. `bundle.sh` packages with
sorted entries, pinned ownership, and a fixed mtime. Golden files under
`internal/render/testdata/golden/` are diffed in CI; refresh them with
`go test ./internal/render/ -run TestGolden -update`.

## Repo layout

```
cmd/                 cobra CLI (render command)
internal/spec/       workload schema, parser, validation
internal/render/     Renderer interface, per-target renderers, embedded templates
internal/fsutil/     deterministic artifact writer
operator/            controller-runtime operator (separate Go module)
workload.yaml        the example spec
```

## Development

```sh
make build      # build the CLI
make test       # unit + golden + validator tests (both modules)
make fmt-check  # gofmt gate
make demo       # render all three targets into _demo/
```
