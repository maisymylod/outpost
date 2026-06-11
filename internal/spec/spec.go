// Package spec defines the outpost workload schema and its parser. It is the
// only package that knows about the on-disk YAML representation; every render
// target consumes the validated Go structs.
package spec

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Known render targets. A spec may request any subset of these.
const (
	TargetCloud  = "cloud"
	TargetOnPrem = "on-prem"
	TargetAirGap = "air-gap"
)

// KnownTargets returns the supported targets in a stable order.
func KnownTargets() []string {
	return []string{TargetCloud, TargetOnPrem, TargetAirGap}
}

// Spec is a single declarative workload definition.
type Spec struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Workload   Workload `yaml:"workload"`
	Targets    []string `yaml:"targets"`
}

// Metadata names and scopes the workload.
type Metadata struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
}

// Workload describes the GPU service to deploy.
type Workload struct {
	Image     string            `yaml:"image"`
	Replicas  int               `yaml:"replicas"`
	GPU       GPU               `yaml:"gpu"`
	Resources Resources         `yaml:"resources"`
	Env       map[string]string `yaml:"env"`
	Ports     []Port            `yaml:"ports"`
	Command   []string          `yaml:"command,omitempty"`
	Args      []string          `yaml:"args,omitempty"`
}

// GPU captures per-replica accelerator requirements and interconnect.
type GPU struct {
	Count        int          `yaml:"count"`
	Class        string       `yaml:"class"`
	Interconnect Interconnect `yaml:"interconnect"`
}

// Interconnect toggles the multi-GPU fabric features to wire up.
type Interconnect struct {
	NCCL       bool `yaml:"nccl"`
	InfiniBand bool `yaml:"infiniband"`
}

// Resources are the non-GPU container resource requests/limits.
type Resources struct {
	CPU       string `yaml:"cpu"`
	Memory    string `yaml:"memory"`
	HugePages string `yaml:"hugepages,omitempty"`
}

// Port is a named container port exposed by the workload.
type Port struct {
	Name string `yaml:"name"`
	Port int    `yaml:"port"`
}

// expectedAPIVersion / expectedKind pin the document shape so a wrong file
// fails loudly instead of rendering garbage.
const (
	expectedAPIVersion = "outpost.dev/v1"
	expectedKind       = "Workload"
)

// dns1123 matches a Kubernetes DNS-1123 label, used for the workload name.
var dns1123 = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// Load parses a spec from r. It does not validate; call Validate separately so
// callers can distinguish parse errors from semantic errors.
func Load(r io.Reader) (*Spec, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read spec: %w", err)
	}
	var s Spec
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("parse spec: %w", err)
	}
	return &s, nil
}

// Validate checks the spec for the invariants every render target relies on.
// It returns the first violation found, in a stable check order.
func (s *Spec) Validate() error {
	if s.APIVersion != expectedAPIVersion {
		return fmt.Errorf("apiVersion: got %q, want %q", s.APIVersion, expectedAPIVersion)
	}
	if s.Kind != expectedKind {
		return fmt.Errorf("kind: got %q, want %q", s.Kind, expectedKind)
	}
	if !dns1123.MatchString(s.Metadata.Name) {
		return fmt.Errorf("metadata.name: %q is not a valid DNS-1123 label", s.Metadata.Name)
	}
	if s.Metadata.Namespace == "" {
		return fmt.Errorf("metadata.namespace: must not be empty")
	}
	if s.Workload.Image == "" {
		return fmt.Errorf("workload.image: must not be empty")
	}
	if s.Workload.Replicas < 1 {
		return fmt.Errorf("workload.replicas: must be >= 1, got %d", s.Workload.Replicas)
	}
	if s.Workload.GPU.Count < 1 {
		return fmt.Errorf("workload.gpu.count: must be >= 1, got %d", s.Workload.GPU.Count)
	}
	if s.Workload.GPU.Class == "" {
		return fmt.Errorf("workload.gpu.class: must not be empty")
	}
	if s.Workload.Resources.CPU == "" || s.Workload.Resources.Memory == "" {
		return fmt.Errorf("workload.resources: cpu and memory are required")
	}
	if s.Workload.GPU.Interconnect.InfiniBand && s.Workload.Resources.HugePages == "" {
		return fmt.Errorf("workload.resources.hugepages: required when infiniband is enabled")
	}
	if len(s.Targets) == 0 {
		return fmt.Errorf("targets: must list at least one of %v", KnownTargets())
	}
	known := map[string]bool{}
	for _, t := range KnownTargets() {
		known[t] = true
	}
	for _, t := range s.Targets {
		if !known[t] {
			return fmt.Errorf("targets: %q is unknown, want subset of %v", t, KnownTargets())
		}
	}
	return nil
}

// WantsTarget reports whether the spec opted into the given render target.
func (s *Spec) WantsTarget(target string) bool {
	for _, t := range s.Targets {
		if t == target {
			return true
		}
	}
	return false
}

// SortedEnv returns env keys in lexical order for deterministic rendering.
func (w Workload) SortedEnv() []string {
	keys := make([]string, 0, len(w.Env))
	for k := range w.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
