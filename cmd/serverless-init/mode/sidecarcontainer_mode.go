// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

//nolint:revive // TODO(SERV) Fix revive linter
package mode

import (
	"os"
	"os/signal"
	"syscall"

	serverlessLog "github.com/DataDog/datadog-agent/cmd/serverless-init/log"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RunSidecar is the entrypoint for sidecar mode. It blocks until a termination
// signal (SIGINT/SIGTERM) is received.
func RunSidecar(_ *serverlessLog.Config) error {
	stopCh := make(chan struct{})
	go handleTerminationSignals(stopCh, signal.Notify)
	<-stopCh
	return nil
}

func handleTerminationSignals(stopCh chan struct{}, notify func(c chan<- os.Signal, sig ...os.Signal)) {
	signalCh := make(chan os.Signal, 1)
	notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	signo := <-signalCh
	log.Infof("Received signal '%s', shutting down...", signo)
	stopCh <- struct{}{}
}
