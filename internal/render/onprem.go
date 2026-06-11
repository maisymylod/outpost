package render

import (
	"embed"

	"github.com/maisymylod/outpost/internal/spec"
)

//go:embed all:templates/onprem
var onPremFS embed.FS

const onPremRoot = "templates/onprem"

// defaultOperatorImage is where the controller image is published. On-prem
// clusters can override it via the chart's values.yaml.
const defaultOperatorImage = "ghcr.io/maisymylod/outpost-operator:v0.1.0"

// chartVersion is the rendered Helm chart version.
const chartVersion = "0.1.0"

// onPremRenderer renders a Helm chart that installs the Workload CRD, the
// controller-runtime operator, and a Workload CR built from the spec. The
// operator reconciles the CR into the GPU-wired Deployment and Service.
type onPremRenderer struct{}

func (onPremRenderer) Target() string { return spec.TargetOnPrem }

// onPremData is the template context for the on-prem chart.
type onPremData struct {
	Spec          *spec.Spec
	OperatorImage string
	ChartVersion  string
	Env           []KV
}

func (onPremRenderer) Render(s *spec.Spec) ([]Artifact, error) {
	data := onPremData{
		Spec:          s,
		OperatorImage: defaultOperatorImage,
		ChartVersion:  chartVersion,
		Env:           sortedKV(s.Workload.Env),
	}

	return renderEmbedded(onPremFS, onPremRoot, "chart", data)
}

func init() {
	Register(onPremRenderer{})
}
