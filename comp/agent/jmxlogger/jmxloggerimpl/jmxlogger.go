// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package jmxloggerimpl implements the logger for JMX.
package jmxloggerimpl

import (
	"context"
	"fmt"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/agent/jmxlogger"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
)

// Module defines the fx options for this component.
func Module(params Params) fxutil.Module {
	return fxutil.Component(
		fx.Provide(newJMXLogger),
		fx.Supply(params),
	)
}

type dependencies struct {
	fx.In
	Lc     fx.Lifecycle
	Config config.Component
	Params Params
}

type jmxLoggerInterface interface {
	Info(v ...interface{})
	Error(v ...interface{}) error
	Flush()
}

type logger struct {
	inner jmxLoggerInterface
}

func newJMXLogger(deps dependencies) (jmxlogger.Component, error) {
	config := deps.Config
	var jmxLogger logger

	if deps.Params.fromCLI {
		i, err := pkglogsetup.BuildJMXLogger(deps.Params.logFile, "", false, true, false, config)
		if err != nil {
			return logger{}, fmt.Errorf("Unable to set up JMX logger: %v", err)
		}

		jmxLogger = logger{
			inner: i,
		}
	} else {
		// Setup logger
		syslogURI := pkglogsetup.GetSyslogURI(config)
		jmxLogFile := config.GetString("jmx_log_file")
		if jmxLogFile == "" {
			jmxLogFile = defaultpaths.JmxLogFile
		}

		if config.GetBool("disable_file_logging") {
			// this will prevent any logging on file
			jmxLogFile = ""
		}

		// Setup JMX logger
		inner, jmxLoggerSetupErr := pkglogsetup.BuildJMXLogger(
			jmxLogFile,
			syslogURI,
			config.GetBool("syslog_rfc"),
			config.GetBool("log_to_console"),
			config.GetBool("log_format_json"),
			config,
		)

		if jmxLoggerSetupErr != nil {
			return logger{}, fmt.Errorf("Error while setting up logging, exiting: %v", jmxLoggerSetupErr)
		}

		jmxLogger = logger{
			inner: inner,
		}
	}

	deps.Lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			jmxLogger.inner.Flush()
			return nil
		},
	})

	return jmxLogger, nil
}

func (j logger) JMXInfo(v ...interface{}) {
	j.inner.Info(v...)
}

func (j logger) JMXError(v ...interface{}) error {
	return j.inner.Error(v...)
}
