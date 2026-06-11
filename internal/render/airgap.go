package render

import (
	"embed"
	"strings"

	"github.com/maisymylod/outpost/internal/spec"
)

//go:embed all:templates/airgap
var airGapFS embed.FS

const airGapRoot = "templates/airgap"

// localRegistry is the in-bundle registry every deployment artifact resolves
// against. It is intentionally a .local name so nothing reaches the internet.
const localRegistry = "registry.outpost.local:5000"

// airGapRenderer emits a self-contained offline bundle: an image manifest for
// mirroring into the local registry, PXE + cloud-init for bootstrapping a
// bare-metal GPU node, a squashfs build script, and a reproducible packager.
type airGapRenderer struct{}

func (airGapRenderer) Target() string { return spec.TargetAirGap }

// mirrorImage describes one image to load into the local registry. The source
// registry host is deliberately absent: the online mirror step supplies it via
// $SOURCE_REGISTRY so the bundle itself names no external registry.
type mirrorImage struct {
	Repository string // host-stripped, e.g. example/vllm-server
	Tag        string
	Archive    string // bundle-relative tar path
	LocalRef   string // registry.outpost.local:5000/example/vllm-server:tag
}

// airGapData is the template context for the air-gap bundle.
type airGapData struct {
	Spec          *spec.Spec
	LocalRegistry string
	Image         mirrorImage
	Env           []KV
}

func (airGapRenderer) Render(s *spec.Spec) ([]Artifact, error) {
	img := mirrorImageFor(s.Workload.Image)
	data := airGapData{
		Spec:          s,
		LocalRegistry: localRegistry,
		Image:         img,
		Env:           deriveEnv(s),
	}
	return renderEmbedded(airGapFS, airGapRoot, "bundle", data)
}

// mirrorImageFor rewrites an upstream image reference into a bundle-local one,
// dropping the source registry host.
func mirrorImageFor(ref string) mirrorImage {
	repo, tag := parseImageRef(ref)
	archive := "images/" + strings.ReplaceAll(repo, "/", "-") + ".tar"
	return mirrorImage{
		Repository: repo,
		Tag:        tag,
		Archive:    archive,
		LocalRef:   localRegistry + "/" + repo + ":" + tag,
	}
}

// parseImageRef splits an image reference into its host-stripped repository and
// tag. A leading segment containing "." or ":" (or "localhost") is treated as
// the registry host and removed.
func parseImageRef(ref string) (repo, tag string) {
	name := ref
	tag = "latest"

	slash := strings.LastIndex(name, "/")
	if colon := strings.LastIndex(name, ":"); colon > slash {
		tag = name[colon+1:]
		name = name[:colon]
	}

	first := name
	rest := ""
	if i := strings.Index(name, "/"); i >= 0 {
		first, rest = name[:i], name[i+1:]
	}
	if strings.ContainsAny(first, ".:") || first == "localhost" {
		return rest, tag
	}
	return name, tag
}

// deriveEnv returns the workload env (user vars sorted, then derived NCCL and
// InfiniBand vars) so the air-gap pod manifest matches what the operator wires.
func deriveEnv(s *spec.Spec) []KV {
	env := sortedKV(s.Workload.Env)
	ic := s.Workload.GPU.Interconnect
	if ic.NCCL {
		env = append(env,
			KV{"NCCL_DEBUG", "INFO"},
			KV{"NCCL_P2P_LEVEL", "NVL"},
			KV{"NCCL_SOCKET_IFNAME", "^lo,docker0"},
		)
	}
	if ic.InfiniBand {
		env = append(env,
			KV{"NCCL_IB_DISABLE", "0"},
			KV{"NCCL_IB_HCA", "mlx5"},
			KV{"NCCL_NET_GDR_LEVEL", "5"},
		)
	}
	return env
}

func init() {
	Register(airGapRenderer{})
}
