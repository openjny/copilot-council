package main

import (
	"github.com/openjny/council/internal/cli"
)

// Version is set at build time via ldflags
var version = "dev"

func main() {
	cli.Execute(version)
}
