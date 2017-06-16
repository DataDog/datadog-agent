package app

import (
	"bytes"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/spf13/cobra"
)

var (
	customerEmail string
	caseID        string
)

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
		config.SetupLogger("off", "")
		if customerEmail == "" && caseID == "" {
			var err error
			customerEmail, err = flare.AskForEmail()
			if err != nil {
				fmt.Println("Error reading email, please retry or contact support")
				os.Exit(1)
			}
		}
		err := requestFlare()
		if err != nil {
			os.Exit(1)
		}
	},
}

func requestFlare() error {
	fmt.Println("Asking the agent to build the flare archive.")
	var e error
	c := GetClient()
	urlstr := "http://" + sockname + "/agent/flare"

	r, e := doPost(c, urlstr, "application/json", bytes.NewBuffer([]byte{}))
	var filePath string
	if e != nil {
		if r != nil && string(r) != "" {
			fmt.Printf("The agent ran into an error while making the flare: %s\n", string(r))
		} else {
			fmt.Println("The agent was unable to make the flare.")
		}
		fmt.Println("Initiating flare locally.")

		filePath, e = flare.CreateArchive(true)
		if e != nil {
			fmt.Printf("The flare zipfile failed to be created: %s\n", e)
			return e
		}
	} else {
		filePath = string(r)
	}

	fmt.Printf("%s is going to be uploaded to Datadog\n", filePath)
	confirmation := flare.AskForConfirmation("Are you sure you want to upload a flare? [Y/N]")
	if !confirmation {
		fmt.Printf("Aborting. (You can still use %s) \n", filePath)
		return nil
	}

	response, e := flare.SendFlare(filePath, caseID, customerEmail)
	fmt.Println(response)
	if e != nil {
		return e
	}
	return nil
}
