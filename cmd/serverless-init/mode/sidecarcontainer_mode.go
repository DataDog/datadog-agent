// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

//nolint:revive // TODO(SERV) Fix revive linter
package mode

import (
	serverlessLog "github.com/DataDog/datadog-agent/cmd/serverless-init/log"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"os"
	"os/signal"
	"syscall"
)

// Run is the entrypoint of the init process. It will spawn the customer process
func RunSidecar(logConfig *serverlessLog.Config) {
	stopCh := make(chan struct{})
	go handleTerminationSignals(stopCh, signal.Notify)
	<-stopCh

}

func handleTerminationSignals(stopCh chan struct{}, notify func(c chan<- os.Signal, sig ...os.Signal)) {
	signalCh := make(chan os.Signal, 1)
	notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	signo := <-signalCh
	log.Infof("Received signal '%s', shutting down...", signo)
	stopCh <- struct{}{}
}
