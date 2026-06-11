// Package cmd wires the outpost cobra command tree.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "outpost",
		Short: "Render one workload spec into cloud, on-prem, and air-gap deployment artifacts",
		Long: "outpost reads a single declarative workload spec and renders deployment\n" +
			"artifacts for multiple targets. One spec in, validated artifacts out:\n" +
			"  cloud    Terraform (managed K8s + GPU node pool) and Helm values\n" +
			"  on-prem  Helm chart, Workload CRD, and a controller-runtime operator\n" +
			"  air-gap  offline bundle: image manifest, PXE, cloud-init, squashfs",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newRenderCmd())
	return root
}

// Execute runs the root command and exits non-zero on error.
func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "outpost: "+err.Error())
		os.Exit(1)
	}
}
