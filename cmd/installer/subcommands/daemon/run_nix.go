// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package daemon

import (
	"errors"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/pid"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/comp/updater/localapi"
	"github.com/DataDog/datadog-agent/comp/updater/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// ErrNotEnabled represents the case in which datadog-installer is not enabled
var ErrNotEnabled = errors.New("datadog-installer not enabled")

func runFxWrapper(global *command.GlobalParams) error {
	return fxutil.OneShot(
		run,
		getCommonFxOption(global),
	)
}

func run(shutdowner fx.Shutdowner, cfg config.Component, _ pid.Component, _ localapi.Component, _ telemetry.Component) error {
	if err := gracefullyExitIfDisabled(cfg, shutdowner); err != nil {
		log.Infof("Datadog installer is not enabled, exiting")
		return nil
	}
	releaseMemory()
	handleSignals(shutdowner)
	return nil
}

func gracefullyExitIfDisabled(cfg config.Component, shutdowner fx.Shutdowner) error {
	if !cfg.GetBool("remote_updates") {
		// Note: when not using systemd we may run into an issue where we need to
		// sleep for a while here, like the system probe does
		// See https://github.com/DataDog/datadog-agent/blob/b5c6a93dff27a8fdae37fc9bf23b3604a9f87591/cmd/system-probe/subcommands/run/command.go#L128
		_ = shutdowner.Shutdown()
		return ErrNotEnabled
	}
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
