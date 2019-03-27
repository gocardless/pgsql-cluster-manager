package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	Version   = "dev"
	Commit    = "none"
	Date      = "unknown"
	GoVersion = runtime.Version()
)

func NewVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version of pgsql-cluster-manager",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf(
				"pgcm Version: %v\nGit SHA: %v\nGo Version: %v\nGo OS/Arch: %v/%v\nBuilt at: %v\n",
				Version, Commit, GoVersion, runtime.GOOS, runtime.GOARCH, Date,
			)
		},
	}
}
