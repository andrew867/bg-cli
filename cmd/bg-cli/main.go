package main

import (
	"os"

	"github.com/andrew867/bg-cli/internal/cli"
)

// version is overridden via -ldflags at release time.
var version = "0.1.4"

func main() {
	cli.Version = version
	os.Exit(cli.Execute(cli.Deps{}, os.Args[1:]))
}
