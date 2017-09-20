package main

import (
	"fmt"
	"os"

	"github.com/gocardless/pgsql-cluster-manager/command"
)

func main() {
	if err := command.PgsqlCommand.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
