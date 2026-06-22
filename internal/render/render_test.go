package render

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// TestCloudTerraform validates the rendered Terraform with the real tool:
// fmt -check (no network) always, and init + validate when network is allowed.
// Skipped when terraform is not installed.
func TestCloudTerraform(t *testing.T) {
	tfBin, err := exec.LookPath("terraform")
	if err != nil {
		t.Skip("terraform not installed; skipping")
	}

	s := loadExample(t)
	arts, err := Render(spec.TargetCloud, s)
	if err != nil {
		t.Fatalf("render cloud: %v", err)
	}

	dir := t.TempDir()
	for _, a := range arts {
		dst := filepath.Join(dir, filepath.FromSlash(a.Path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, a.Content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tfDir := filepath.Join(dir, "terraform")

	// fmt -check is offline and fast; always run it.
	if out, err := runIn(tfDir, tfBin, "fmt", "-check", "-recursive"); err != nil {
		t.Fatalf("terraform fmt -check failed: %v\n%s", err, out)
	}

	if os.Getenv("OUTPOST_SKIP_TERRAFORM_INIT") != "" {
		t.Skip("OUTPOST_SKIP_TERRAFORM_INIT set; ran fmt -check only")
	}

	if out, err := runIn(tfDir, tfBin, "init", "-backend=false", "-input=false"); err != nil {
		t.Fatalf("terraform init failed: %v\n%s", err, out)
	}
	if out, err := runIn(tfDir, tfBin, "validate"); err != nil {
		t.Fatalf("terraform validate failed: %v\n%s", err, out)
	}
}

func runIn(dir, bin string, args ...string) ([]byte, error) {
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TF_IN_AUTOMATION=1")
	return cmd.CombinedOutput()
}

// externalRegistry matches public container registry hosts that must never
// appear in the air-gap deploy path.
var externalRegistry = regexp.MustCompile(`(?i)\b(docker\.io|registry-1\.docker\.io|ghcr\.io|quay\.io|gcr\.io|registry\.k8s\.io|k8s\.gcr\.io|nvcr\.io|mcr\.microsoft\.com|public\.ecr\.aws|[a-z0-9-]+\.amazonaws\.com)\b`)

// urlRef matches a URL and captures its host (with optional port).
var urlRef = regexp.MustCompile(`(?i)\b(?:https?|ftp)://([a-z0-9._-]+(?::[0-9]+)?)`)

// isLocalHost reports whether a URL host is internal to the air-gapped network.
func isLocalHost(host string) bool {
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	if host == "localhost" || strings.HasSuffix(host, ".local") {
		return true
	}
	for _, p := range []string{"10.", "192.168.", "127."} {
		if strings.HasPrefix(host, p) {
			return true
		}
	}
	return false
}

// TestAirGapOfflinePurity fails if any rendered air-gap artifact references an
// external registry or a non-local URL. This is the core air-gap invariant.
func TestAirGapOfflinePurity(t *testing.T) {
	s := loadExample(t)
	arts, err := Render(spec.TargetAirGap, s)
	if err != nil {
		t.Fatalf("render air-gap: %v", err)
	}
	if len(arts) == 0 {
		t.Fatal("air-gap produced no artifacts")
	}

	for _, a := range arts {
		for i, line := range strings.Split(string(a.Content), "\n") {
			lineNo := i + 1
			if loc := externalRegistry.FindString(line); loc != "" {
				t.Errorf("%s:%d references external registry %q: %s", a.Path, lineNo, loc, strings.TrimSpace(line))
			}
			for _, m := range urlRef.FindAllStringSubmatch(line, -1) {
				if !isLocalHost(m[1]) {
					t.Errorf("%s:%d references non-local URL host %q: %s", a.Path, lineNo, m[1], strings.TrimSpace(line))
				}
			}
		}
	}
}

// TestAirGapShellcheck runs shellcheck over the rendered bundle scripts. It is
// skipped when shellcheck is not installed.
func TestAirGapShellcheck(t *testing.T) {
	scBin, err := exec.LookPath("shellcheck")
	if err != nil {
		t.Skip("shellcheck not installed; skipping")
	}
	s := loadExample(t)
	arts, err := Render(spec.TargetAirGap, s)
	if err != nil {
		t.Fatalf("render air-gap: %v", err)
	}

	dir := t.TempDir()
	var scripts []string
	for _, a := range arts {
		if !strings.HasSuffix(a.Path, ".sh") {
			continue
		}
		dst := filepath.Join(dir, filepath.FromSlash(a.Path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, a.Content, 0o755); err != nil {
			t.Fatal(err)
		}
		scripts = append(scripts, dst)
	}
	if len(scripts) == 0 {
		t.Fatal("no shell scripts rendered for air-gap")
	}

	args := append([]string{"--severity=warning"}, scripts...)
	if out, err := exec.Command(scBin, args...).CombinedOutput(); err != nil {
		t.Fatalf("shellcheck failed:\n%s", out)
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

func TestResolveGPUInstance(t *testing.T) {
	// A known class maps cleanly with no warning.
	if got, warn := resolveGPUInstance("nvidia-h100-80gb"); got != "p5.48xlarge" || warn != "" {
		t.Fatalf("known class resolved to (%q, %q), want (p5.48xlarge, \"\")", got, warn)
	}

	// An unknown class falls back to the default and warns, naming the class,
	// the default it picked, and at least one known class.
	got, warn := resolveGPUInstance("nvidia-b200")
	if got != defaultGPUInstanceType {
		t.Errorf("unknown class instance = %q, want default %q", got, defaultGPUInstanceType)
	}
	if warn == "" {
		t.Fatal("expected a warning for an unknown GPU class, got none")
	}
	for _, want := range []string{"nvidia-b200", defaultGPUInstanceType, "nvidia-h100-80gb"} {
		if !strings.Contains(warn, want) {
			t.Errorf("warning %q does not mention %q", warn, want)
		}
	}
}

func TestMirrorImageForValidatesRepo(t *testing.T) {
	// A normal upstream ref host-strips cleanly into a valid bundle image.
	img, err := mirrorImageFor("ghcr.io/example/vllm-server:0.6.3")
	if err != nil {
		t.Fatalf("valid ref rejected: %v", err)
	}
	if img.Repository != "example/vllm-server" || img.Tag != "0.6.3" {
		t.Errorf("parsed (%q, %q), want (example/vllm-server, 0.6.3)", img.Repository, img.Tag)
	}
	if img.Archive != "images/example-vllm-server.tar" {
		t.Errorf("archive = %q, want images/example-vllm-server.tar", img.Archive)
	}

	// Refs that would yield a broken archive path or local ref must be rejected.
	for _, bad := range []string{
		"ghcr.io/ex ample/vllm-server", // space
		"registry:5000/",               // empty repo after host strip
		"ghcr.io/UPPER/name",           // uppercase is not a valid repo
	} {
		if _, err := mirrorImageFor(bad); err == nil {
			t.Errorf("expected an error for %q, got none", bad)
		}
	}
}
