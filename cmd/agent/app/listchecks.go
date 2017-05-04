package app

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	AgentCmd.AddCommand(listCheckCommand)

}

var listCheckCommand = &cobra.Command{
	Use:   "listchecks",
	Short: "Query the running agent for the hostname.",
	Long:  ``,
	RunE:  doListChecks,
}

// query for the version
func doListChecks(cmd *cobra.Command, args []string) error {
	c := GetClient()
	urlstr := "http://" + sockname + "/check/"

	body, e := doGet(c, urlstr)
	if e != nil {
		fmt.Printf("Error getting version string: %s\n", e)
		return e
	}
	fmt.Printf("Checks: %s\n", body)
	return nil
}
