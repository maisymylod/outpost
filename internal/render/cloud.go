package render

import (
	"embed"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/maisymylod/outpost/internal/spec"
)

//go:embed all:templates/cloud
var cloudFS embed.FS

const cloudRoot = "templates/cloud"

// gpuInstanceTypes maps a GPU class to an AWS EC2 instance type whose
// accelerators match. The fallback covers unknown classes with a capable
// default so the rendered Terraform is always valid.
var gpuInstanceTypes = map[string]string{
	"nvidia-a100-80gb": "p4de.24xlarge",
	"nvidia-a100-40gb": "p4d.24xlarge",
	"nvidia-h100-80gb": "p5.48xlarge",
	"nvidia-a10g":      "g5.12xlarge",
	"nvidia-l4":        "g6.12xlarge",
}

const defaultGPUInstanceType = "p4de.24xlarge"

// resolveGPUInstance maps a GPU class to an EC2 instance type. When the class
// is unknown it falls back to the default and returns a non-empty warning
// describing the substitution (the unknown class, the chosen default, and the
// known classes) so the caller can surface it rather than silently shipping a
// possibly-wrong accelerator.
func resolveGPUInstance(class string) (instance, warning string) {
	if instance, ok := gpuInstanceTypes[class]; ok {
		return instance, ""
	}
	known := make([]string, 0, len(gpuInstanceTypes))
	for k := range gpuInstanceTypes {
		known = append(known, k)
	}
	sort.Strings(known)
	warning = fmt.Sprintf("unknown GPU class %q; using default instance %s (known classes: %s)",
		class, defaultGPUInstanceType, strings.Join(known, ", "))
	return defaultGPUInstanceType, warning
}

// cloudRenderer emits Terraform for a managed EKS cluster with a GPU node group
// plus Helm values for deploying the workload onto it.
type cloudRenderer struct{}

func (cloudRenderer) Target() string { return spec.TargetCloud }

// cloudData is the template context for the cloud target.
type cloudData struct {
	Spec            *spec.Spec
	ClusterName     string
	GPUInstanceType string
	NodeDesired     int
	NodeMin         int
	NodeMax         int
	OperatorImage   string
	Env             []KV
}

func (cloudRenderer) Render(s *spec.Spec) ([]Artifact, error) {
	instance, warning := resolveGPUInstance(s.Workload.GPU.Class)
	if warning != "" {
		fmt.Fprintln(os.Stderr, "outpost: "+warning)
	}

	// One replica per GPU node; allow headroom for rolling updates.
	desired := s.Workload.Replicas
	data := cloudData{
		Spec:            s,
		ClusterName:     s.Metadata.Name,
		GPUInstanceType: instance,
		NodeDesired:     desired,
		NodeMin:         desired,
		NodeMax:         desired * 2,
		OperatorImage:   defaultOperatorImage,
		Env:             sortedKV(s.Workload.Env),
	}

	return renderEmbedded(cloudFS, cloudRoot, ".", data)
}

func init() {
	Register(cloudRenderer{})
}
