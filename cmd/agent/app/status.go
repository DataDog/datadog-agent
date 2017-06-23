package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/spf13/cobra"
)

var (
	jsonStatus      bool
	prettyPrintJSON bool
)

func init() {
	AgentCmd.AddCommand(statusCmd)
	statusCmd.Flags().StringVarP(&confFilePath, "cfgpath", "f", "", "path to folder containing datadog.yaml")
	statusCmd.Flags().BoolVarP(&jsonStatus, "json", "j", false, "print out raw json")
	statusCmd.Flags().BoolVarP(&prettyPrintJSON, "pretty-json", "p", false, "pretty print JSON")
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Print the current status",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		common.SetupConfig(confFilePath)
		err := requestStatus()
		if err != nil {
			fmt.Printf("Error Printing Status: %v", err)
		}
	},
}

func requestStatus() error {
	fmt.Fprintf(os.Stderr, "Getting the status from the agent.\n\n")
	var e error
	c := GetClient()
	urlstr := "http://" + sockname + "/agent/status"

	r, e := doGet(c, urlstr)
	if e != nil {
		fmt.Printf("Could not reach agent: %v. \n\n Make sure the agent is running before requesting the status and contact support if you continue having issues. \n", e)
		return e
	}
	if prettyPrintJSON {
		var prettyJSON bytes.Buffer
		json.Indent(&prettyJSON, r, "", "  ")
		fmt.Println(prettyJSON.String())
	} else if jsonStatus {
		fmt.Println(string(r))
	} else {
		formattedStatus, err := status.FormatStatus(r)
		if err != nil {
			return err
		}
		fmt.Printf(formattedStatus)
	}

	return nil
}
