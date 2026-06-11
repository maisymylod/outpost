package spec

import (
	"os"
	"strings"
	"testing"
)

// validSpec is a minimal spec that passes Validate; tests mutate copies of it.
const validSpec = `
apiVersion: outpost.dev/v1
kind: Workload
metadata:
  name: llm-inference
  namespace: inference
workload:
  image: ghcr.io/example/vllm-server:0.6.3
  replicas: 2
  gpu:
    count: 4
    class: nvidia-a100-80gb
    interconnect:
      nccl: true
      infiniband: true
  resources:
    cpu: "16"
    memory: 128Gi
    hugepages: 2Gi
  env:
    MODEL_ID: demo
  ports:
    - name: http
      port: 8000
targets:
  - cloud
  - on-prem
  - air-gap
`

func TestLoad(t *testing.T) {
	s, err := Load(strings.NewReader(validSpec))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Metadata.Name != "llm-inference" {
		t.Errorf("name = %q, want llm-inference", s.Metadata.Name)
	}
	if s.Workload.GPU.Count != 4 {
		t.Errorf("gpu.count = %d, want 4", s.Workload.GPU.Count)
	}
	if !s.Workload.GPU.Interconnect.InfiniBand {
		t.Error("expected infiniband enabled")
	}
	if got, want := len(s.Workload.Ports), 1; got != want {
		t.Errorf("ports = %d, want %d", got, want)
	}
	if got := s.Workload.Env["MODEL_ID"]; got != "demo" {
		t.Errorf("env MODEL_ID = %q, want demo", got)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	in := validSpec + "\nbogusTopLevel: nope\n"
	if _, err := Load(strings.NewReader(in)); err == nil {
		t.Fatal("expected error on unknown field, got nil")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Spec)
		wantErr string // substring; "" means expect success
	}{
		{"valid", func(*Spec) {}, ""},
		{"bad apiVersion", func(s *Spec) { s.APIVersion = "v1" }, "apiVersion"},
		{"bad kind", func(s *Spec) { s.Kind = "Pod" }, "kind"},
		{"bad name", func(s *Spec) { s.Metadata.Name = "Bad_Name" }, "DNS-1123"},
		{"empty namespace", func(s *Spec) { s.Metadata.Namespace = "" }, "namespace"},
		{"empty image", func(s *Spec) { s.Workload.Image = "" }, "image"},
		{"zero replicas", func(s *Spec) { s.Workload.Replicas = 0 }, "replicas"},
		{"zero gpus", func(s *Spec) { s.Workload.GPU.Count = 0 }, "gpu.count"},
		{"empty gpu class", func(s *Spec) { s.Workload.GPU.Class = "" }, "gpu.class"},
		{"missing cpu", func(s *Spec) { s.Workload.Resources.CPU = "" }, "resources"},
		{"ib without hugepages", func(s *Spec) { s.Workload.Resources.HugePages = "" }, "hugepages"},
		{"no targets", func(s *Spec) { s.Targets = nil }, "targets"},
		{"unknown target", func(s *Spec) { s.Targets = []string{"moon"} }, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Load(strings.NewReader(validSpec))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			tt.mutate(s)
			err = s.Validate()
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("Validate: unexpected error %v", err)
			case tt.wantErr != "" && err == nil:
				t.Fatalf("Validate: expected error containing %q, got nil", tt.wantErr)
			case tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr):
				t.Fatalf("Validate: error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestWantsTarget(t *testing.T) {
	s, err := Load(strings.NewReader(validSpec))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !s.WantsTarget(TargetOnPrem) {
		t.Error("expected on-prem target")
	}
	if s.WantsTarget("moon") {
		t.Error("did not expect moon target")
	}
}

func TestSortedEnv(t *testing.T) {
	w := Workload{Env: map[string]string{"B": "2", "A": "1", "C": "3"}}
	got := w.SortedEnv()
	want := []string{"A", "B", "C"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("SortedEnv = %v, want %v", got, want)
	}
}

// TestExampleSpec ensures the committed example spec stays valid.
func TestExampleSpec(t *testing.T) {
	f, err := os.Open("../../workload.yaml")
	if err != nil {
		t.Fatalf("open example: %v", err)
	}
	defer f.Close()
	s, err := Load(f)
	if err != nil {
		t.Fatalf("load example: %v", err)
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("example spec invalid: %v", err)
	}
}
