// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "trace-agent",
	Short: "trace-agent is Datadog's APM agent",
	Long: `
trace-agent is Datadog's APM agent. It is comprised of an HTTP API which processes incoming
payloads representing tracing data. It augments them with stats and extracts Trace Search &
Analytics events, forwarding them to the Datadog API. The trace-agent is usually bundled and
run with the Datadog Agent package.`,
	SilenceUsage: true,
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&agent.ConfigPath, "cfgpath", "c", DefaultConfigPath, "path to directory containing datadog.yaml")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(-1)
	}
}

// handleSignal closes a channel to exit cleanly from routines
func handleSignal(onSignal func()) {
	sigChan := make(chan os.Signal, 10)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGPIPE)
	for signo := range sigChan {
		switch signo {
		case syscall.SIGINT, syscall.SIGTERM:
			log.Infof("received signal %d (%v)", signo, signo)
			onSignal()
			return
		case syscall.SIGPIPE:
			// By default systemd redirects the stdout to journald. When journald is stopped or crashes we receive a SIGPIPE signal.
			// Go ignores SIGPIPE signals unless it is when stdout or stdout is closed, in this case the agent is stopped.
			// We never want the agent to stop upon receiving SIGPIPE, so we intercept the SIGPIPE signals and just discard them.
		default:
			log.Warnf("unhandled signal %d (%v)", signo, signo)
		}
	}
}
