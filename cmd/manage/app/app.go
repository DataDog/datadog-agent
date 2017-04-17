/*
Package app implements the Agent main loop, orchestrating
all the components and providing the command line interface.
*/
package app

import "github.com/spf13/cobra"

var (
	// flags variables
	sockname    string
	checkname   string
	deletecheck bool

	// ManageCmd is the root command
	ManageCmd = &cobra.Command{
		Use:   "manage [command]",
		Short: "Datadog Agent service manager.",
		Long: `
The Datadog Agent service manager is a command line tool for controlling the DataDog agent.`,
	}
)

func init() {
	ManageCmd.Flags().StringVarP(&sockname, "name", "n", "agent.sock", "name of socket/pipe")
}
