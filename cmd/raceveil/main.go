// Command raceveil is RaceVeil's CLI entrypoint. It performs no logic of its
// own — command definitions live in internal/presentation/cli, and their
// dependencies are assembled in internal/wiring (docs/SYSTEM_DESIGN.md §2).
package main

import (
	"os"

	"github.com/Lakshya5876/Raceveil/internal/presentation/cli"
)

func main() {
	os.Exit(cli.Execute())
}
