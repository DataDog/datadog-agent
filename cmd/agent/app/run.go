// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package app

import (
	"fmt"
	"syscall"

	_ "expvar"         // Blank import used because this isn't directly used in this file
	_ "net/http/pprof" // Blank import used because this isn't directly used in this file

	"os"
	"os/signal"

	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	log "github.com/cihub/seelog"
	"github.com/spf13/cobra"
)

var (
	runCmd = &cobra.Command{
		Use:   "run",
		Short: "Run the Agent",
		Long:  `Runs the agent in the foreground`,
		RunE:  run,
	}
)

var (
	// flags variables
	runForeground bool
	pidfilePath   string
)

// run the host metadata collector every 14400 seconds (4 hours)
const hostMetadataCollectorInterval = 14400

// run the agent checks metadata collector every 600 seconds (10 minutes)
const agentChecksMetadataCollectorInterval = 600

// run the resources metadata collector every 300 seconds (5 minutes) by default, configurable
const defaultResourcesMetadataCollectorInterval = 300

func init() {

	// attach the command to the root
	AgentCmd.AddCommand(runCmd)

	// local flags
	runCmd.Flags().StringVarP(&pidfilePath, "pidfile", "p", "", "path to the pidfile")
}

// Start the main loop
func run(cmd *cobra.Command, args []string) error {
	defer func() {
		StopAgent()
	}()

	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// Make a channel to exit the function
	stopCh := make(chan error)

	go func() {
		// Set up the signals async so we can Start the agent
		select {
		case <-signals.Stopper:
			log.Info("Received stop command, shutting down...")
			stopCh <- nil
		case <-signals.ErrorStopper:
			log.Critical("The Agent has encountered an error, shutting down...")
			stopCh <- fmt.Errorf("shutting down because of an error")
		case sig := <-signalCh:
			log.Infof("Received signal '%s', shutting down...", sig)
			stopCh <- nil
		}
	}()

	if err := StartAgent(); err != nil {
		return err
	}

	select {
	case err := <-stopCh:
		return err
	}
}
