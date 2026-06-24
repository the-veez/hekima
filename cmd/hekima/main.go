// Hekima — domain-specific document chunker for East African AI systems.
// See docs/architecture.md for the full design.
package main

import (
	"fmt"
	"os"

	"github.com/the-veez/hekima/internal/cli"
	"github.com/the-veez/hekima/internal/server"
)

func main() {
	// --serve mode: hekima --serve [--port 8080]
	// All other invocations go to the CLI.
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "--serve" {
		port := "8080"
		for i, arg := range args {
			if arg == "--port" && i+1 < len(args) {
				port = args[i+1]
			}
		}
		if err := server.Run(":" + port); err != nil {
			fmt.Fprintf(os.Stderr, "hekima: server error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := cli.Run(args); err != nil {
		fmt.Fprintf(os.Stderr, "hekima: %v\n", err)
		os.Exit(1)
	}
}
