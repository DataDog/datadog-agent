package app

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	AgentCmd.AddCommand(flareCmd)
}

var flareCmd = &cobra.Command{
	Use:   "flare",
	Short: "Collect a flare and send it to Datadog (FIXME: NYI)",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(`I dunno how to make a flare ¯\_(ツ)_/¯`)
	},
}
