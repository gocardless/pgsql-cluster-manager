package main

import (
	"os"

	"github.com/gocardless/pgsql-cluster-manager/command"
)

func main() {
	if err := command.PgsqlCommand.Execute(); err != nil {
		os.Exit(1)
	}
}
