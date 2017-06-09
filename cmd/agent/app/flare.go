package app

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
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
		if customerEmail == "" {
			customerEmail = flare.AskForEmail()
		}
		err := requestFlare()
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println(`I dunno how to make a flare ¯\_(ツ)_/¯`)
	},
}

func requestFlare() error {
	c := GetClient()
	urlstr := "http://" + sockname + "/agent/flare"
	var e error
	var postbody = make(map[string]string)
	postbody["case_id"] = caseID
	postbody["email"] = customerEmail
	body, _ := json.Marshal(postbody)

	_, e = doPost(c, urlstr, "application/json", bytes.NewBuffer(body))
	if e != nil {
		fmt.Println("Unable to contact agent; initiating flare locally")
		filePath, err := flare.CreateArchive()
		if err != nil {
			fmt.Printf("The flare zipfile failed to be created: %s\n", err)
			return err
		}
		err = flare.SendFlare(filePath, caseID, customerEmail)
		if err != nil {
			fmt.Printf("The flare failed to send: %s\n", err)
			return err
		}
	}
	return nil
}
