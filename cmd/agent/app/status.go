package app

import (
	"bytes"
	"encoding/json"
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
	fmt.Println("Asking the agent to build the flare archive.")
	var e error
	c := GetClient()
	urlstr := "http://" + sockname + "/agent/status"

	r, e := doGet(c, urlstr)
	if e != nil {
		return e
	}
	var statuses = make(map[string]string)
	json.Unmarshal(r, &statuses)
	fmt.Println("Status: ")
	for name, status := range statuses {
		// j, _ := json.Marshal(status)
		var prettyJ bytes.Buffer
		json.Indent(&prettyJ, []byte(status), "", "    ")
		fmt.Printf("%s:\n", name)
		fmt.Println(string(prettyJ.Bytes()))
		fmt.Println("")
	}

	return nil
}
