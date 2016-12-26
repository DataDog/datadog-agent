package app

import (
	"github.com/DataDog/datadog-agent/cmd/agent/app/ipc"
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

	c, err := ipc.GetConn()
	if err != nil {
		panic(err)
	}
	defer c.Close()

	_, err = c.Write([]byte("stop"))
	if err != nil {
		log.Errorf("Error sending stop command: %v", err)
	}
}
