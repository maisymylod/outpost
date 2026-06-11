package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/maisymylod/outpost/internal/fsutil"
	"github.com/maisymylod/outpost/internal/render"
	"github.com/maisymylod/outpost/internal/spec"
	"github.com/spf13/cobra"
)

func newRenderCmd() *cobra.Command {
	var (
		specPath string
		outDir   string
		targets  []string
		airGap   bool
	)

	cmd := &cobra.Command{
		Use:   "render",
		Short: "Render artifacts for one or more targets from a workload spec",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := loadSpec(specPath)
			if err != nil {
				return err
			}

			selected, err := resolveTargets(s, targets, airGap)
			if err != nil {
				return err
			}

			for _, t := range selected {
				arts, err := render.Render(t, s)
				if err != nil {
					return err
				}
				dst := filepath.Join(outDir, t)
				if err := os.RemoveAll(dst); err != nil {
					return fmt.Errorf("clean %s: %w", dst, err)
				}
				if err := fsutil.Write(dst, arts); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "rendered %s: %d artifacts -> %s\n", t, len(arts), dst)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&specPath, "spec", "f", "workload.yaml", "path to the workload spec")
	cmd.Flags().StringVarP(&outDir, "out", "o", "out", "output directory (one subdir per target)")
	cmd.Flags().StringSliceVarP(&targets, "target", "t", nil, "target(s) to render; defaults to the spec's targets list")
	cmd.Flags().BoolVar(&airGap, "air-gap", false, "shorthand for --target air-gap")
	return cmd
}

// loadSpec reads and validates the spec at path.
func loadSpec(path string) (*spec.Spec, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open spec: %w", err)
	}
	defer f.Close()

	s, err := spec.Load(f)
	if err != nil {
		return nil, err
	}
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("invalid spec %s: %w", path, err)
	}
	return s, nil
}

// resolveTargets decides which targets to render from the flags and spec,
// rejecting any target the spec did not opt into.
func resolveTargets(s *spec.Spec, flagTargets []string, airGap bool) ([]string, error) {
	want := append([]string{}, flagTargets...)
	if airGap {
		want = append(want, spec.TargetAirGap)
	}
	if len(want) == 0 {
		want = append(want, s.Targets...)
	}

	seen := map[string]bool{}
	var out []string
	for _, t := range want {
		if seen[t] {
			continue
		}
		if !s.WantsTarget(t) {
			return nil, fmt.Errorf("target %q is not listed in the spec's targets (%v)", t, s.Targets)
		}
		seen[t] = true
		out = append(out, t)
	}
	sort.Strings(out)
	return out, nil
}
