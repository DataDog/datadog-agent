/*
Package app implements the Agent main loop, orchestrating
all the components and providing the command line interface.
*/
package app

import "github.com/spf13/cobra"

var (
	// AgentCmd is the root command
	AgentCmd = &cobra.Command{
		Use:   "agent [command]",
		Short: "Datadog Agent at your service.",
		Long: `
The Datadog Agent faithfully collects events and metrics and brings them 
to Datadog on your behalf so that you can do something useful with your 
monitoring and performance data.`,
	}

	// flags variables
	sockname  string
	checkname string
)

func init() {
	AgentCmd.Flags().StringVarP(&sockname, "name", "n", "agent.sock", "name of socket/pipe")
}
