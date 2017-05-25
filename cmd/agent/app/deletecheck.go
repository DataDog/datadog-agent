package app

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	AgentCmd.AddCommand(deleteCheckCommand)
	deleteCheckCommand.Flags().StringVarP(&checkname, "checkname", "c", "", "name of check")
}

var deleteCheckCommand = &cobra.Command{
	Use:   "deletecheck",
	Short: "Delete the given check",
	Long:  ``,
	RunE:  doDeleteCheck,
}

// query for the version
func doDeleteCheck(cmd *cobra.Command, args []string) error {

	if len(checkname) == 0 {
		return fmt.Errorf("Must supply a check name to query")
	}
	c := GetClient()
	urlstr := "http://" + sockname + "/check/" + checkname
	var e error
	var body []byte
	// todo, change to DELETE
	body, e = doGet(c, urlstr)

	if e != nil {
		fmt.Printf("Error deleting check  %s: %s\n", checkname, e)
		return e
	}
	fmt.Printf("Check %s deleted: %s\n", checkname, body)
	return nil
}
