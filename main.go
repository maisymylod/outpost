// Command outpost renders a single declarative workload spec into deployment
// artifacts for multiple targets: cloud, on-prem, and air-gapped bare-metal.
package main

import "github.com/maisymylod/outpost/cmd"

func main() {
	cmd.Execute()
}
