package app

import (
	"syscall"

	"os"
	"os/signal"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"
	"github.com/spf13/cobra"
)

var (
	startCmd = &cobra.Command{
		Use:   "start",
		Short: "Start the Agent",
		Long:  `Runs the agent in the foreground`,
		Run:   start,
	}
)

func init() {
	// attach the command to the root
	AgentCmd.AddCommand(startCmd)

	// local flags
	startCmd.Flags().StringVarP(&pidfilePath, "pidfile", "p", "", "path to the pidfile")
	startCmd.Flags().StringVarP(&confdPath, "confd", "c", "", "path to the confd folder")
	startCmd.Flags().StringVarP(&confFilePath, "cfgpath", "f", "", "path to directory containing datadog.yaml")
	config.Datadog.BindPFlag("confd_path", startCmd.Flags().Lookup("confd"))
}

// Start the main loop
func start(cmd *cobra.Command, args []string) {
	StartAgent()
	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGINT)

	// Block here until we receive the interrupt signal
	select {
	case <-common.Stopper:
		log.Info("Received stop command, shutting down...")
	case sig := <-signalCh:
		log.Infof("Received signal '%s', shutting down...", sig)
	}
	StopAgent()
}
