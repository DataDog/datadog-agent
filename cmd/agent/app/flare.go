package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/flare"
	"github.com/spf13/cobra"
)

var customerEmail string
var caseID string

func init() {
	AgentCmd.AddCommand(flareCmd)

	flareCmd.Flags().StringVarP(&customerEmail, "email", "e", "", "Your email")
	flareCmd.Flags().StringVarP(&caseID, "case-id", "c", "", "Your case ID")
	flareCmd.Flags().StringVarP(&confFilePath, "cfgpath", "f", "", "path to datadog.yaml")
	flareCmd.SetArgs([]string{"caseID"})
}

var flareCmd = &cobra.Command{
	Use:   "flare [caseID]",
	Short: "Collect a flare and send it to Datadog (FIXME: NYI)",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		common.SetupConfig(confFilePath)
		// The flare command should not log anything, all errors should be reported directly to the console without the log format
		config.SetupLogger("error", "")
		if customerEmail == "" && caseID == "" {
			customerEmail = flare.AskForEmail()
		}
		err := requestFlare()
		if err != nil {
			os.Exit(1)
		}
	},
}

func requestFlare() error {
	fmt.Println("Building the flare archive")
	var e error
	c := GetClient()
	urlstr := "http://" + sockname + "/agent/flare"
	var postbody = make(map[string]string)
	postbody["case_id"] = caseID
	postbody["email"] = customerEmail
	body, _ := json.Marshal(postbody)

	r, e := doPost(c, urlstr, "application/json", bytes.NewBuffer(body))
	var filePath string
	filePath = string(r)
	if e != nil {
		fmt.Println("Unable to contact agent; initiating flare locally")
		filePath, e = flare.CreateArchive()
		if e != nil {
			fmt.Printf("The flare zipfile failed to be created: %s\n", e)
			return e
		}
	}

	fmt.Printf("%s is going to be uploaded to Datadog\n", filePath)
	confirmation := flare.AskForConfirmation("Are you sure you want to upload a flare? [Y/N]")
	if !confirmation {
		fmt.Printf("Aborting. (You can still use %s) \n", filePath)
		return nil
	}

	e = flare.SendFlare(filePath, caseID, customerEmail)
	if e != nil {
		return e
	}
	return nil
}
