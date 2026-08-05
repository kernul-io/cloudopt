package main

import (
	"os"

	"github.com/kernul-io/cloudopt/internal/adapters/cli"
)

func main() {
	cfg := &cli.Config{}
	os.Exit(cli.Execute(cfg))
}
