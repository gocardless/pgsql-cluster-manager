package main

import (
	"fmt"
	"os"

	"github.com/gocardless/pgsql-cluster-manager/cmd"
)

func main() {
	if err := cmd.PgsqlCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
