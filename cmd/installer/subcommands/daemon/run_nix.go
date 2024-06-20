// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package daemon

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/comp/core/pid"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/updater/localapi"
	"github.com/DataDog/datadog-agent/comp/updater/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func runFxWrapper(global *command.GlobalParams) error {
	return fxutil.OneShot(
		run,
		getCommonFxOption(global),
	)
}

func run(shutdowner fx.Shutdowner, _ pid.Component, _ localapi.Component, _ telemetry.Component) error {
	handleSignals(shutdowner)
	return nil
}

func handleSignals(shutdowner fx.Shutdowner) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGPIPE)
	for signo := range sigChan {
		switch signo {
		case syscall.SIGINT, syscall.SIGTERM:
			log.Infof("Received signal %d (%v)", signo, signo)
			_ = shutdowner.Shutdown()
			return
		}
	}
}
