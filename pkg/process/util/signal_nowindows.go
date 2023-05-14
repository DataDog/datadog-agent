// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package util

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// HandleSignals tells us whether we should exit.
func HandleSignals(exit chan struct{}) {
	sigIn := make(chan os.Signal, 100)
	signal.Notify(sigIn, syscall.SIGINT, syscall.SIGTERM, syscall.SIGPIPE)
	// unix only in all likelihood; but we don't care.
	for sig := range sigIn {
		switch sig {
		case syscall.SIGINT, syscall.SIGTERM:
			log.Infof("Caught signal '%s'; terminating.", sig)
			close(exit)
			return
		case syscall.SIGPIPE:
			// By default systemd redirects the stdout to journald. When journald is stopped or crashes we receive a SIGPIPE signal.
			// Go ignores SIGPIPE signals unless it is when stderr or stdout is closed, in this case the agent is stopped.
			// We never want the agent to stop upon receiving SIGPIPE, so we intercept the SIGPIPE signals and just discard them.
			// See https://golang.org/pkg/os/signal/#hdr-SIGPIPE
			continue
		}
	}
}
