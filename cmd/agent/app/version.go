package app

import (
	"fmt"

	"github.com/spf13/cobra"
)

const agentVersion = "6.0.0"

func init() {
	AgentCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(fmt.Sprintf("Agent %s - Codename: Χελωνη", agentVersion))
	},
}
