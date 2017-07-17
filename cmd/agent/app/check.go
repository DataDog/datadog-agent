package app

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	checkRate int
	checkName string
)

func init() {
	AgentCmd.AddCommand(checkCmd)

	checkCmd.Flags().StringVarP(&confFilePath, "cfgpath", "f", "", "path to datadog.yaml")
	checkCmd.Flags().IntVarP(&checkRate, "check-rate", "r", 0, "check rate")
	checkCmd.Flags().StringVarP(&checkName, "check", "c", "", "check name")
	checkCmd.SetArgs([]string{"checkName"})
}

var checkCmd = &cobra.Command{
	Use:   "check -c <check_name>",
	Short: "Run the specified check",
	Long:  `Use this to run a specific check with a specific rate`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) != 0 {
			checkName = args[0]
		}
		fmt.Println(checkName)
	},
}
