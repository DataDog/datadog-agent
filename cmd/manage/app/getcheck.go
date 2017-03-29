package app

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	ManageCmd.AddCommand(getCheckCommand)
	getCheckCommand.Flags().StringVarP(&checkname, "checkname", "c", "", "name of check")
	getCheckCommand.Flags().BoolVarP(&deletecheck, "delete", "d", false, "Delete the check")
}

var getCheckCommand = &cobra.Command{
	Use:   "getcheck",
	Short: "Query the running agent for the status of a given check.",
	Long:  ``,
	RunE:  dogetCheck,
}

// query for the version
func dogetCheck(cmd *cobra.Command, args []string) error {

	if len(checkname) == 0 {
		return fmt.Errorf("Must supply a check name to query")
	}
	c := GetClient()
	urlstr := "http://" + sockname + "/check/" + checkname
	var e error
	var body []byte
	if deletecheck {
		// todo, change to DELETE
		body, e = doGet(c, urlstr)
	} else {
		body, e = doGet(c, urlstr)
	}

	if e != nil {
		fmt.Printf("Error getting check status for check %s: %s\n", checkname, e)
		return e
	}
	fmt.Printf("Check %s status: %s\n", checkname, body)
	return nil
}
