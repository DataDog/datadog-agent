package app

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/spf13/cobra"
)

func init() {
	AgentCmd.AddCommand(statusCmd)
	statusCmd.Flags().StringVarP(&confFilePath, "cfgpath", "f", "", "path to datadog.yaml")
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

	fmt.Println(string(r))

	return nil
}
