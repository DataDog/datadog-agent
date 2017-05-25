package app

import (
	"fmt"

	"strings"

	"github.com/spf13/cobra"
)

func init() {
	AgentCmd.AddCommand(reloadCheckCommand)
	reloadCheckCommand.Flags().StringVarP(&checkname, "checkname", "c", "", "name of check")
}

var reloadCheckCommand = &cobra.Command{
	Use:   "reloadCheck",
	Short: "Reload a running check.",
	Long:  ``,
	RunE:  doreloadCheck,
}

// query for the version
func doreloadCheck(cmd *cobra.Command, args []string) error {

	if len(checkname) == 0 {
		return fmt.Errorf("Must supply a check name to query")
	}
	c := GetClient()
	urlstr := "http://" + sockname + "/check/" + checkname + "/reload"

	postbody := ""

	body, e := doPost(c, urlstr, "application/json", strings.NewReader(postbody))

	if e != nil {
		fmt.Printf("Error getting check status for check %s: %s\n", checkname, e)
		return e
	}
	fmt.Printf("Reload check %s: %s\n", checkname, body)
	return nil
}
