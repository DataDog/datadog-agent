// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package daemon

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	logdef "github.com/DataDog/datadog-agent/comp/core/log/def"
	sysprobeconfigimpl "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/impl"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// macOSFleetEnabledKey opts a macOS host into running the installer daemon. Every .dmg install
// loads the daemon's LaunchDaemon, but the daemon only does anything once this is set.
const macOSFleetEnabledKey = "installer.macos_fleet.enabled"

// daemonEnabledOnPlatform reports whether this platform lets the daemon run with the given
// configuration. On macOS it has to be opted into (macOSFleetEnabledKey).
func daemonEnabledOnPlatform(cfg model.Reader) bool {
	return cfg.GetBool(macOSFleetEnabledKey)
}

// daemonEnabled reports whether the daemon should run with the given configuration.
func daemonEnabled(cfg model.Reader) bool {
	return cfg.GetBool("remote_updates") && daemonEnabledOnPlatform(cfg)
}

// daemonEnabledBeforeStart loads only the configuration and reports whether the daemon should run,
// so a host that has not opted in exits before any component that needs remote configuration or
// the run directory is built. Every .dmg install loads the daemon's job, and launchd relaunches it
// whenever it exits unsuccessfully, so a component failing to build there would loop.
func daemonEnabledBeforeStart(global *command.GlobalParams) (enabled bool, err error) {
	err = fxutil.OneShot(func(cfg config.Component, logger logdef.Component) {
		enabled = daemonEnabled(cfg)
		if !enabled {
			logger.Infof("Datadog installer is not enabled on this platform, exiting")
		}
	},
		fx.Supply(core.BundleParams{
			ConfigParams:         config.NewAgentParams(global.ConfFilePath),
			SysprobeConfigParams: sysprobeconfigimpl.NewParams(),
			LogParams:            logdef.ForDaemon("INSTALLER", "installer.log_file", defaultpaths.GetDefaultUpdaterLogFile()),
		}),
		core.Bundle(core.WithSecrets()),
	)
	return enabled, err
}
