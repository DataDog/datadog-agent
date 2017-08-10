/*
Package app implements the Agent main loop, orchestrating
all the components and providing the command line interface.
*/
package app

import (
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"
	"github.com/spf13/cobra"
)

var (
	// AgentCmd is the root command
	AgentCmd = &cobra.Command{
		Use:   "agent [command]",
		Short: "Datadog Agent at your service.",
		Long: `
The Datadog Agent faithfully collects events and metrics and brings them
to Datadog on your behalf so that you can do something useful with your
monitoring and performance data.`,
	}

	// flags variables
	sockname  string
	checkname string
)

func init() {
	var defaultSockName string
	if runtime.GOOS == "windows" {
		defaultSockName = strings.SplitAfter(config.Datadog.GetString("cmd_pipe_name"), "pipe\\")[1]
		log.Debugf("Set defaultSockName to %s\n", defaultSockName)
	} else {
		defaultSockName = strings.SplitAfter(config.Datadog.GetString("cmd_sock"), "tmp/")[1]
	}
	AgentCmd.Flags().StringVarP(&sockname, "name", "n", defaultSockName, "name of socket/pipe")
}
