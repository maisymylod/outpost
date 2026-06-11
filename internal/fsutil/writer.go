// Package fsutil writes rendered artifacts to disk deterministically: stable
// ordering, fixed file modes, and no timestamps embedded in the tree.
package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/maisymylod/outpost/internal/render"
)

// Write materializes artifacts under root. It creates parent directories as
// needed and writes files in sorted path order. An existing root is reused;
// callers that want a clean tree should remove it first.
func Write(root string, arts []render.Artifact) error {
	sorted := make([]render.Artifact, len(arts))
	copy(sorted, arts)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })

	for _, a := range sorted {
		dst := filepath.Join(root, filepath.FromSlash(a.Path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("mkdir for %s: %w", a.Path, err)
		}
		mode := a.Mode
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(dst, a.Content, mode); err != nil {
			return fmt.Errorf("write %s: %w", a.Path, err)
		}
	}
	return nil
}
