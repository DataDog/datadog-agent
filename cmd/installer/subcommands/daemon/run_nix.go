// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package daemon

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/pid"
	"github.com/DataDog/datadog-agent/comp/updater/localapi"
	telemetry "github.com/DataDog/datadog-agent/comp/updater/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func runFxWrapper(global *command.GlobalParams) error {
	return fxutil.Run(
		getCommonFxOption(global),
		fx.Invoke(func(_ pid.Component) {}),
		fx.Invoke(func(_ localapi.Component) {}),
		fx.Invoke(func(_ telemetry.Component) {}),
		fx.Invoke(startDaemon),
	)
}

func startDaemon(shutdowner fx.Shutdowner, cfg config.Component) {
	if !cfg.GetBool("remote_updates") {
		log.Infof("Datadog installer is not enabled, exiting")
		_ = shutdowner.Shutdown()
		return
	}
	releaseMemory()
}
