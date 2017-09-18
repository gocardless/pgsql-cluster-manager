package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var Version string // set at compile-time

func NewVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Prints the version of pgsql-cluster-manager",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("pgsql-cluster-manager: %s\n", Version)
		},
	}
}
