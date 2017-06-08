package app

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/spf13/cobra"
)

func init() {
	AgentCmd.AddCommand(flareCmd)
}

var flareCmd = &cobra.Command{
	Use:   "flare",
	Short: "Collect a flare and send it to Datadog (FIXME: NYI)",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		common.SetupConfig("")
		doFlare()
		fmt.Println(`I dunno how to make a flare ¯\_(ツ)_/¯`)
	},
}

func doFlare() {
	filePath, err := util.CreateArchive()
	fmt.Println("filePath", filePath)
	fmt.Println("error", err)
	if err != nil {
		fmt.Errorf("Error sending Flare: ", err)
	}
	// err = util.SendFlare(filePath, "", "", "")
	if err != nil {
		fmt.Errorf("Error sending Flare: ", err)
	}

	fmt.Println("I made a flare here: ", filePath)
	dir, _ := os.Getwd()

	exec.Command("cp", filePath, dir).Run()
	os.Remove(filePath)
}
