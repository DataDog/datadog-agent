package app

import "github.com/spf13/cobra"

var (
	stopCmd = &cobra.Command{
		Use:   "stop",
		Short: "Stop the Agent",
		Long:  ``,
		Run:   stop,
	}
)

func stop(*cobra.Command, []string) {

}
