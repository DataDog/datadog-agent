// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/DataDog/datadog-agent/cmd/security-agent/app/common"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/spf13/cobra"
)

type startCliParams struct {
	*common.GlobalParams

	pidfilePath string
}

func StartCommands(globalParams *common.GlobalParams) []*cobra.Command {
	cliParams := startCliParams{
		GlobalParams: globalParams,
	}

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Security Agent",
		Long:  `Runs Datadog Security agent in the foreground`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return start(&cliParams)
		},
	}

	startCmd.Flags().StringVarP(&cliParams.pidfilePath, "pidfile", "p", "", "path to the pidfile")

	return []*cobra.Command{startCmd}
}

func start(cliParams *startCliParams) error {
	// Main context passed to components
	ctx, cancel := context.WithCancel(context.Background())
	defer StopAgent(cancel)

	err := RunAgent(ctx, cliParams.pidfilePath)
	if errors.Is(err, errAllComponentsDisabled) {
		return nil
	}
	if err != nil {
		return err
	}

	stopCh := make(chan struct{})
	defer close(stopCh)
	go handleSignals(stopCh)

	// Block here until we receive a stop signal
	<-stopCh

	return nil
}

// handleSignals handles OS signals, and sends a message on stopCh when an interrupt
// signal is received.
func handleSignals(stopCh chan struct{}) {
	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM, syscall.SIGPIPE)

	// Block here until we receive the interrupt signal
	for signo := range signalCh {
		switch signo {
		case syscall.SIGPIPE:
			// By default systemd redirects the stdout to journald. When journald is stopped or crashes we receive a SIGPIPE signal.
			// Go ignores SIGPIPE signals unless it is when stdout or stdout is closed, in this case the agent is stopped.
			// We never want dogstatsd to stop upon receiving SIGPIPE, so we intercept the SIGPIPE signals and just discard them.
		default:
			log.Infof("Received signal '%s', shutting down...", signo)

			_ = tagger.Stop()

			stopCh <- struct{}{}
			return
		}
	}
}
