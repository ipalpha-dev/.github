// Command ipalpha is the IPAlpha developer tool: setup, run, pull, publish, feature, doctor…
// One self-contained binary per OS; the workspace wrappers (./run, run.cmd…) call it.
package main

import (
	"os"

	"github.com/ipalpha-dev/tooling/internal/cli"
)

func main() { os.Exit(cli.Main(os.Args[1:])) }
