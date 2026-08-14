// Command threatoptic is the ThreatOptic command-line client.
package main

import (
	"os"

	"github.com/ThreatOptic/CLI/internal/cmd"
)

// version is overridden at build time with -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	if err := cmd.NewRoot(version).Execute(); err != nil {
		os.Exit(1)
	}
}
