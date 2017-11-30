package main

import (
	"os"

	"github.com/gocardless/pgsql-cluster-manager/command"
)

var Version string // set at compile-time

func main() {
	if err := command.PgsqlCommand.Execute(); err != nil {
		os.Exit(1)
	}
}
