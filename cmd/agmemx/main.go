package main

import (
	"os"

	"agmemx/internal/cli"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Environ(), cwd))
}
