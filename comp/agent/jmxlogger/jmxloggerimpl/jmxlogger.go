// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package jmxloggerimpl implements the logger for JMX.
package jmxloggerimpl

import (
	"fmt"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/comp/agent/jmxlogger"
	"github.com/DataDog/datadog-agent/comp/core/config"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newJMXLogger),
	)
}

type dependencies struct {
	fx.In
	Config config.Component
	Params Params
}

type logger struct{}

func newJMXLogger(deps dependencies) (jmxlogger.Component, error) {
	config := deps.Config
	if deps.Params.disabled {
		return logger{}, nil
	}
	if deps.Params.fromCLI {
		err := pkgconfig.SetupJMXLogger(deps.Params.logFile, "", false, true, false)
		if err != nil {
			err = fmt.Errorf("Unable to set up JMX logger: %v", err)
		}
		return logger{}, err
	}

	// Setup logger
	syslogURI := pkgconfig.GetSyslogURI()
	jmxLogFile := config.GetString("jmx_log_file")
	if jmxLogFile == "" {
		jmxLogFile = path.DefaultJmxLogFile
	}

	if config.GetBool("disable_file_logging") {
		// this will prevent any logging on file
		jmxLogFile = ""
	}

	// Setup JMX logger
	jmxLoggerSetupErr := pkgconfig.SetupJMXLogger(
		jmxLogFile,
		syslogURI,
		config.GetBool("syslog_rfc"),
		config.GetBool("log_to_console"),
		config.GetBool("log_format_json"),
	)

	if jmxLoggerSetupErr != nil {
		jmxLoggerSetupErr = fmt.Errorf("Error while setting up logging, exiting: %v", jmxLoggerSetupErr)
	}
	return logger{}, jmxLoggerSetupErr
}

func (j logger) JMXInfo(v ...interface{}) {
	log.JMXInfo(v...)
}

func (j logger) JMXError(v ...interface{}) error {
	return log.JMXError(v...)
}
