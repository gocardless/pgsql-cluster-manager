package command

import (
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var migrateLongDescription = `
Talk to the migration API- hosted on the Postgres nodes- in order to cause the
Postgres primary to be migrated to the current sync node.

This migration is performed by first pausing PGBouncer, waiting until all
queries have finished, then performing a pacemaker resource migration. Once the
migration is complete, we issue a PGBouncer resume, restoring traffic to the
cluster.
`

func NewMigrateCommand() *cobra.Command {
	cm := &cobra.Command{
		Use:   "migrate",
		Short: "Run a zero-downtime migration of Postgres primary",
		Long:  migrateLongDescription,
		Run:   migrateCommandFunc,
	}

	flags := cm.Flags()

	flags.StringSlice("migration-api-endpoints", []string{"http://pg01:8080", "http://pg02:8080", "http://pg03:8080"}, "All Postgres node API endpoints")
	flags.Duration("pause-timeout", 5*time.Second, "Timeout for all nodes to pause PGBouncer")
	flags.Duration("pause-expiry", 25*time.Second, "Time after which PGBouncer will automatically lift pause")

	viper.BindPFlag("migration-api-endpoints", flags.Lookup("migration-api-endpoints"))
	viper.BindPFlag("pause-timeout", flags.Lookup("pause-timeout"))
	viper.BindPFlag("pause-expiry", flags.Lookup("pause-expiry"))

	return cm
}

func migrateCommandFunc(cmd *cobra.Command, args []string) {
}
