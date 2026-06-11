package render

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/maisymylod/outpost/internal/spec"
	"gopkg.in/yaml.v3"
)

// update regenerates the committed golden renders instead of diffing them.
var update = flag.Bool("update", false, "update golden files")

// loadExample parses and validates the repo's example workload spec.
func loadExample(t *testing.T) *spec.Spec {
	t.Helper()
	f, err := os.Open("../../workload.yaml")
	if err != nil {
		t.Fatalf("open example spec: %v", err)
	}
	defer f.Close()
	s, err := spec.Load(f)
	if err != nil {
		t.Fatalf("load example spec: %v", err)
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("example spec invalid: %v", err)
	}
	return s
}

// TestGolden renders every target from the example spec and diffs the result
// against the committed golden tree. Run with -update to refresh the goldens.
func TestGolden(t *testing.T) {
	s := loadExample(t)
	for _, target := range Registered() {
		t.Run(target, func(t *testing.T) {
			arts, err := Render(target, s)
			if err != nil {
				t.Fatalf("render %s: %v", target, err)
			}
			goldenDir := filepath.Join("testdata", "golden", target)
			if *update {
				writeGolden(t, goldenDir, arts)
				return
			}
			compareGolden(t, goldenDir, arts)
		})
	}
}

// TestDeterministic renders each target twice and requires byte-identical output.
func TestDeterministic(t *testing.T) {
	s := loadExample(t)
	for _, target := range Registered() {
		t.Run(target, func(t *testing.T) {
			a, err := Render(target, s)
			if err != nil {
				t.Fatalf("render a: %v", err)
			}
			b, err := Render(target, s)
			if err != nil {
				t.Fatalf("render b: %v", err)
			}
			if len(a) != len(b) {
				t.Fatalf("artifact count differs: %d vs %d", len(a), len(b))
			}
			for i := range a {
				if a[i].Path != b[i].Path {
					t.Fatalf("path %d differs: %q vs %q", i, a[i].Path, b[i].Path)
				}
				if !bytes.Equal(a[i].Content, b[i].Content) {
					t.Fatalf("content for %q is not deterministic", a[i].Path)
				}
			}
		})
	}
}

func writeGolden(t *testing.T, dir string, arts []Artifact) {
	t.Helper()
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("clean golden dir: %v", err)
	}
	for _, a := range arts {
		dst := filepath.Join(dir, filepath.FromSlash(a.Path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(dst, a.Content, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}
	t.Logf("updated golden tree %s (%d files)", dir, len(arts))
}

func compareGolden(t *testing.T, dir string, arts []Artifact) {
	t.Helper()
	got := map[string][]byte{}
	for _, a := range arts {
		got[filepath.ToSlash(a.Path)] = a.Content
	}

	// Collect the golden files on disk.
	want := map[string][]byte{}
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		want[filepath.ToSlash(rel)] = data
		return nil
	})
	if err != nil {
		t.Fatalf("walk golden dir %s: %v (run with -update to create)", dir, err)
	}

	for path, w := range want {
		g, ok := got[path]
		if !ok {
			t.Errorf("golden has %q but render did not produce it", path)
			continue
		}
		if !bytes.Equal(g, w) {
			t.Errorf("content mismatch for %q (run with -update if intended)", path)
		}
	}
	for path := range got {
		if _, ok := want[path]; !ok {
			t.Errorf("render produced %q but it is not in golden (run with -update)", path)
		}
	}
}

// TestOnPremCRDStructure validates the rendered CRD without a cluster or any
// external dependency: it must be a well-formed CustomResourceDefinition with
// the Workload schema the operator expects.
func TestOnPremCRDStructure(t *testing.T) {
	s := loadExample(t)
	arts, err := Render(spec.TargetOnPrem, s)
	if err != nil {
		t.Fatalf("render on-prem: %v", err)
	}
	crd := findArtifact(t, arts, "chart/crds/workload.outpost.dev.yaml")

	var doc struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
		Spec       struct {
			Group string `yaml:"group"`
			Names struct {
				Kind   string `yaml:"kind"`
				Plural string `yaml:"plural"`
			} `yaml:"names"`
			Versions []struct {
				Name    string `yaml:"name"`
				Served  bool   `yaml:"served"`
				Storage bool   `yaml:"storage"`
				Schema  struct {
					OpenAPIV3Schema map[string]any `yaml:"openAPIV3Schema"`
				} `yaml:"schema"`
			} `yaml:"versions"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(crd, &doc); err != nil {
		t.Fatalf("parse CRD: %v", err)
	}

	if doc.APIVersion != "apiextensions.k8s.io/v1" {
		t.Errorf("CRD apiVersion = %q", doc.APIVersion)
	}
	if doc.Kind != "CustomResourceDefinition" {
		t.Errorf("CRD kind = %q", doc.Kind)
	}
	if doc.Spec.Group != "outpost.dev" || doc.Spec.Names.Kind != "Workload" {
		t.Errorf("CRD group/kind = %q/%q", doc.Spec.Group, doc.Spec.Names.Kind)
	}
	if len(doc.Spec.Versions) != 1 {
		t.Fatalf("CRD versions = %d, want 1", len(doc.Spec.Versions))
	}
	v := doc.Spec.Versions[0]
	if v.Name != "v1" || !v.Served || !v.Storage {
		t.Errorf("CRD version = %+v, want served+storage v1", v)
	}
	if len(v.Schema.OpenAPIV3Schema) == 0 {
		t.Error("CRD is missing an openAPIV3Schema")
	}
}

// TestOnPremHelm validates the rendered chart with the real tools. It is
// skipped when helm or kubeconform are not installed.
func TestOnPremHelm(t *testing.T) {
	helmBin, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm not installed; skipping")
	}
	kcBin, kcErr := exec.LookPath("kubeconform")

	s := loadExample(t)
	arts, err := Render(spec.TargetOnPrem, s)
	if err != nil {
		t.Fatalf("render on-prem: %v", err)
	}

	dir := t.TempDir()
	chartDir := filepath.Join(dir, "chart")
	for _, a := range arts {
		dst := filepath.Join(dir, filepath.FromSlash(a.Path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, a.Content, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// helm lint
	if out, err := exec.Command(helmBin, "lint", chartDir).CombinedOutput(); err != nil {
		t.Fatalf("helm lint failed: %v\n%s", err, out)
	}

	// helm template --include-crds piped to kubeconform
	tmplOut, err := exec.Command(helmBin, "template", "rel", chartDir, "--namespace", s.Metadata.Namespace, "--include-crds").CombinedOutput()
	if err != nil {
		t.Fatalf("helm template failed: %v\n%s", err, tmplOut)
	}
	if kcErr != nil {
		t.Skip("kubeconform not installed; helm checks passed")
	}
	cmd := exec.Command(kcBin, "-strict", "-ignore-missing-schemas", "-summary")
	cmd.Stdin = bytes.NewReader(tmplOut)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("kubeconform failed: %v\n%s", err, out)
	}
}

func findArtifact(t *testing.T, arts []Artifact, path string) []byte {
	t.Helper()
	for _, a := range arts {
		if a.Path == path {
			return a.Content
		}
	}
	var have []string
	for _, a := range arts {
		have = append(have, a.Path)
	}
	sort.Strings(have)
	t.Fatalf("artifact %q not found; have %s", path, strings.Join(have, ", "))
	return nil
}
