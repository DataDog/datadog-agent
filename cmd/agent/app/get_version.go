package app

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	AgentCmd.AddCommand(getVersionCommand)

}

var getVersionCommand = &cobra.Command{
	Use:   "getversion",
	Short: "Query the running agent for the software version.",
	Long:  ``,
	RunE:  doGetVersion,
}

// query for the version
func doGetVersion(cmd *cobra.Command, args []string) error {
	c := GetClient()
	urlstr := "http://" + sockname + "/agent/version"

	body, e := doGet(c, urlstr)
	if e != nil {
		fmt.Printf("Error getting version string: %s\n", e)
		return e
	}
	fmt.Printf("Version: %s\n", body)
	return nil
}
