package app

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/spf13/cobra"
)

// AgentVersion is the reference version number
const agentVersion = "6.0.0+Χελωνη"

func init() {
	AgentCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		av, _ := version.New(agentVersion)
		fmt.Println(fmt.Sprintf("Agent %s - Codename: %s", av.GetNumber(), av.Meta))
	},
}
