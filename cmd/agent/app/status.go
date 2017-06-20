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
	statusCmd.Flags().StringVarP(&confFilePath, "cfgpath", "f", "", "path to datadog.yaml")
	statusCmd.Flags().BoolVarP(&jsonStatus, "json", "j", false, "print out raw json")
	statusCmd.Flags().BoolVarP(&prettyPrintJSON, "pretty-print-json", "p", false, "print out raw json")
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Print the current status (FIXME: NYI)",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		common.SetupConfig(confFilePath)
		requestStatus()
	},
}

func requestStatus() error {
	fmt.Fprintf(os.Stderr, "Getting the status from the agent.\n\n")
	// fmt.Errorf("Getting the status from the agent.\n\n")
	// fmt.Printf("Getting the status from the agent.\n\n")
	var e error
	c := GetClient()
	urlstr := "http://" + sockname + "/agent/status"

	r, e := doGet(c, urlstr)
	if e != nil {
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
