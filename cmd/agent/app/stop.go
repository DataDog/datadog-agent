package app

import (
	"github.com/DataDog/datadog-agent/cmd/agent/app/api"
	log "github.com/cihub/seelog"
	"github.com/spf13/cobra"
)

var (
	stopCmd = &cobra.Command{
		Use:   "stop",
		Short: "Stop the Agent",
		Long:  ``,
		Run:   stop,
	}
)

func init() {
	// attach the command to the root
	AgentCmd.AddCommand(stopCmd)
}

func stop(*cobra.Command, []string) {
	// Global Agent configuration
	setupConfig()

	c := api.GetClient()
	resp, err := c.Post("http://localhost/agent/stop", "", nil)
	if err != nil || resp.StatusCode != 202 {
		log.Errorf("Error sending stop command: %v", err)
	}
}
