// Hekima — domain-specific document chunker for East African AI systems.
// See docs/architecture.md for the full design.
package main

import (
	"fmt"
	"os"

	"github.com/the-veez/hekima/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "hekima: %v\n", err)
		os.Exit(1)
	}
}
