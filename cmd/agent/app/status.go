package app

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/spf13/cobra"
)

var noFormatStatus bool

func init() {
	AgentCmd.AddCommand(statusCmd)
	statusCmd.Flags().StringVarP(&confFilePath, "cfgpath", "f", "", "path to datadog.yaml")
	statusCmd.Flags().BoolVarP(&noFormatStatus, "no-format", "j", false, "print out raw json")
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
	fmt.Printf("Getting the status from the agent.\n\n")
	var e error
	c := GetClient()
	urlstr := "http://" + sockname + "/agent/status"

	r, e := doGet(c, urlstr)
	if e != nil {
		return e
	}
	if noFormatStatus {
		stats := make(map[string]string)
		json.Unmarshal(r, &stats)
		for key, value := range stats {
			fmt.Printf("%v: %v\n\n", key, value)
		}
	} else {
		formattedStatus, err := status.FormatStatus(r)
		if err != nil {
			return err
		}
		fmt.Printf(formattedStatus)
	}

	return nil
}
