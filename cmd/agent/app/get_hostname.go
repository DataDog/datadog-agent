package app

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	AgentCmd.AddCommand(getHostnameCommand)

}

var getHostnameCommand = &cobra.Command{
	Use:   "gethostname",
	Short: "Query the running agent for the hostname.",
	Long:  ``,
	RunE:  doGetHostname,
}

// query for the version
func doGetHostname(cmd *cobra.Command, args []string) error {
	c := GetClient()
	urlstr := "http://" + sockname + "/agent/hostname"

	body, e := doGet(c, urlstr)
	if e != nil {
		fmt.Printf("Error getting version string: %s\n", e)
		return e
	}
	fmt.Printf("Hostname: %s\n", body)
	return nil
}
