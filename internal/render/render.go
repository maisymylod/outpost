// Package render turns a validated spec into deployment artifacts. Each target
// implements Renderer and registers itself; the CLI dispatches by target name.
// Output is deterministic: a given spec always yields byte-identical artifacts.
package render

import (
	"fmt"
	"io/fs"
	"sort"

	"github.com/maisymylod/outpost/internal/spec"
)

// Artifact is a single rendered file: its path relative to the target output
// directory, its file mode, and its content.
type Artifact struct {
	Path    string
	Mode    fs.FileMode
	Content []byte
}

// Renderer renders one target's artifacts from a validated spec.
type Renderer interface {
	// Target is the stable target name, e.g. "cloud".
	Target() string
	// Render produces the artifacts. Implementations must be deterministic.
	Render(*spec.Spec) ([]Artifact, error)
}

// registry holds the renderer for each target name.
var registry = map[string]Renderer{}

// Register makes a renderer available by its target name. It panics on a
// duplicate registration, which can only be a programming error.
func Register(r Renderer) {
	t := r.Target()
	if _, dup := registry[t]; dup {
		panic(fmt.Sprintf("render: duplicate renderer for target %q", t))
	}
	registry[t] = r
}

// For returns the renderer registered for target, if any.
func For(target string) (Renderer, bool) {
	r, ok := registry[target]
	return r, ok
}

// Registered returns the registered target names in stable order.
func Registered() []string {
	out := make([]string, 0, len(registry))
	for t := range registry {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// Render resolves the renderer for target and runs it. Artifacts come back
// sorted by path so downstream writers and golden tests see a stable order.
func Render(target string, s *spec.Spec) ([]Artifact, error) {
	r, ok := For(target)
	if !ok {
		return nil, fmt.Errorf("no renderer registered for target %q (have %v)", target, Registered())
	}
	arts, err := r.Render(s)
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", target, err)
	}
	sort.Slice(arts, func(i, j int) bool { return arts[i].Path < arts[j].Path })
	return arts, nil
}
