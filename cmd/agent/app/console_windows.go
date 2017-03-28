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
	consoleCmd = &cobra.Command{
		Use:   "console",
		Short: "Run the agent as a console application",
		Long:  ``,
		RunE:  console,
	}
)

func init() {
	// attach the command to the root
	AgentCmd.AddCommand(consoleCmd)

	// Global Agent configuration
	common.SetupConfig()

	consoleCmd.Flags().StringVarP(&confdPath, "confd", "c", "", "path to the confd folder")
	config.Datadog.BindPFlag("confd_path", consoleCmd.Flags().Lookup("confd"))
}

// Start the main loop
func console(cmd *cobra.Command, args []string) error {
	statsd, collector, forwarder := StartAgent()
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
	StopAgent(statsd, collector, forwarder)
	return nil
}
