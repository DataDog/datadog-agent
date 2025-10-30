// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive
package main

import (
	"context"
	"os"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secretsfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	privateactionrunnerfx "github.com/DataDog/datadog-agent/comp/privateactionrunner/fx"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient/rcclientimpl"
	commonsettings "github.com/DataDog/datadog-agent/pkg/config/settings"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
	"go.uber.org/fx"
)

const (
	loggerName     pkglogsetup.LoggerName = "DD_EXTENSION"
	logLevelEnvVar                        = "DD_LOG_LEVEL"
)

func main() {
	// run the agent
	err := fxutil.OneShot(
		runPar,
		// Provide the required modules with their dependencies
		fx.Supply(
			fx.Annotate(
				context.Background(),
				fx.As(new(context.Context)),
			),
		),
		// Supply BundleParams for core.Bundle()
		fx.Supply(core.BundleParams{
			ConfigParams:         config.NewAgentParams(""),
			SysprobeConfigParams: sysprobeconfigimpl.NewParams(),
			LogParams:            log.ForOneShot("privateactionrunner", "info", true),
		}),
		// Use core.Bundle() instead of individual modules
		core.Bundle(),
		secretsfx.Module(),
		// Add settings parameters provider
		fx.Provide(func(config config.Component) settings.Params {
			return settings.Params{
				Settings: map[string]settings.RuntimeSetting{
					"log_level": commonsettings.NewLogLevelRuntimeSetting(),
				},
				Config: config,
			}
		}),
		// Add required modules for rcclient
		settingsimpl.Module(),
		ipcfx.ModuleReadWrite(),
		// Provide the remote config client module
		rcclientimpl.Module(),
		// Finally, provide the privateactionrunner module
		privateactionrunnerfx.Module(),
	)

	if err != nil {
		pkglog.Error(err)
		os.Exit(-1)
	}
}

func runPar(par privateactionrunner.Component, logger log.Component, _ config.Component, _ rcclient.Component) {
	setupLogger()

	err := par.Start(context.Background())
	if err != nil {
		logger.Error(err)
		os.Exit(-1)
	}
}

func setupLogger() {
	logLevel := "error"
	if userLogLevel := os.Getenv(logLevelEnvVar); len(userLogLevel) > 0 {
		if seelogLogLevel, err := pkglog.ValidateLogLevel(userLogLevel); err == nil {
			logLevel = seelogLogLevel
		} else {
			pkglog.Errorf("Invalid log level '%s', using default log level '%s'", userLogLevel, logLevel)
		}
	}

	// init the logger configuring it to not log in a file (the first empty string)
	if err := pkglogsetup.SetupLogger(
		loggerName,
		logLevel,
		"",    // logFile -> by setting this to an empty string, we don't write the logs to any file
		"",    // syslog URI
		false, // syslog_rfc
		true,  // log_to_console
		false, // log_format_json
		pkgconfigsetup.Datadog(),
	); err != nil {
		pkglog.Errorf("Unable to setup logger: %s", err)
	}
}
