package main

import (
	"context"

	"github.com/gocardless/pgsql-cluster-manager/pkg/cmd"
)

func main() {
	cmd.Execute(context.Background())
}
