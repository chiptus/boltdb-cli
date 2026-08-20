package cmd

import (
	"fmt"

	"github.com/chiptus/boltdb-cli/internal/portainer"
	"github.com/spf13/cobra"
)

func newPortainerCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "portainer",
		Short: "Commands that know Portainer's version-bucket schema",
	}
	c.AddCommand(
		newPortainerGetVersionCmd(),
		newPortainerSetVersionCmd(),
		newPortainerClearUpdatingFlagCmd(),
	)
	return c
}

func newPortainerGetVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get-version",
		Short: "Print the stored Portainer schema version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := resolveDBPath(cmd)
			if err != nil {
				return err
			}
			v, err := portainer.GetVersion(path)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "SchemaVersion: %s\nMigratorCount: %d\nEdition: %d\nInstanceID: %s\n",
				v.SchemaVersion, v.MigratorCount, v.Edition, v.InstanceID)
			return err
		},
	}
}

func newPortainerSetVersionCmd() *cobra.Command {
	var schemaVersion string
	var edition, migratorCount int
	wf := &writeFlags{}

	c := &cobra.Command{
		Use:   "set-version",
		Short: "Patch the stored Portainer schema version, edition, or migrator count",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := resolveDBPath(cmd)
			if err != nil {
				return err
			}

			input := portainer.SetVersionInput{}
			if cmd.Flags().Changed("schema-version") {
				input.SchemaVersion = &schemaVersion
			}
			if cmd.Flags().Changed("edition") {
				input.Edition = &edition
			}
			if cmd.Flags().Changed("migrator-count") {
				input.MigratorCount = &migratorCount
			}

			_, err = portainer.SetVersion(path, input, wf.writeOptions(cmd))
			return err
		},
	}
	c.Flags().StringVar(&schemaVersion, "schema-version", "", "new SchemaVersion (must be valid semver)")
	c.Flags().IntVar(&edition, "edition", 0, "new Edition value")
	c.Flags().IntVar(&migratorCount, "migrator-count", 0, "new MigratorCount value")
	wf.register(c)
	return c
}

func newPortainerClearUpdatingFlagCmd() *cobra.Command {
	wf := &writeFlags{}

	c := &cobra.Command{
		Use:   "clear-updating-flag",
		Short: "Unstick a database left with DB_UPDATING=true after a crashed migration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := resolveDBPath(cmd)
			if err != nil {
				return err
			}
			_, err = portainer.ClearUpdatingFlag(path, wf.writeOptions(cmd))
			return err
		},
	}
	wf.register(c)
	return c
}
