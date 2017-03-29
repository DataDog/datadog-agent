package app

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/spf13/cobra"
)

func init() {
	AgentCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		av, _ := version.New(version.AgentVersion)
		fmt.Println(fmt.Sprintf("Agent %s - Codename: %s - Commit: %s", av.GetNumber(), av.Meta, av.Commit))
	},
}
