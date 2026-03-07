// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package jmxloggerimpl implements the logger for JMX.
package jmxloggerimpl

import (
	"context"
	"fmt"

	jmxlogger "github.com/DataDog/datadog-agent/comp/agent/jmxlogger/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
)

// Requires defines the dependencies for the jmxlogger component.
type Requires struct {
	Lc     compdef.Lifecycle
	Config config.Component
	Params Params
}

// Provides defines the output of the jmxlogger component.
type Provides struct {
	Comp jmxlogger.Component
}

type jmxLoggerInterface interface {
	Info(v ...interface{})
	Error(v ...interface{}) error
	Flush()
	Close()
}

type logger struct {
	inner jmxLoggerInterface
}

// NewComponent creates a new jmxlogger component.
func NewComponent(reqs Requires) (Provides, error) {
	var inner jmxLoggerInterface
	var err error

	if reqs.Params.fromCLI {
		inner, err = pkglogsetup.BuildJMXLogger(reqs.Params.logFile, "", false, true, false, reqs.Config)
		if err != nil {
			return Provides{}, fmt.Errorf("Unable to set up JMX logger: %v", err)
		}
	} else {
		syslogURI := pkglogsetup.GetSyslogURI(reqs.Config)
		jmxLogFile := reqs.Config.GetString("jmx_log_file")
		if jmxLogFile == "" {
			jmxLogFile = defaultpaths.JmxLogFile
		}

		if reqs.Config.GetBool("disable_file_logging") {
			// this will prevent any logging on file
			jmxLogFile = ""
		}

		inner, err = pkglogsetup.BuildJMXLogger(
			jmxLogFile,
			syslogURI,
			reqs.Config.GetBool("syslog_rfc"),
			reqs.Config.GetBool("log_to_console"),
			reqs.Config.GetBool("log_format_json"),
			reqs.Config,
		)

		if err != nil {
			return Provides{}, fmt.Errorf("Error while setting up logging, exiting: %v", err)
		}
	}

	jmxLogger := logger{
		inner: inner,
	}

	reqs.Lc.Append(compdef.Hook{
		OnStop: func(_ context.Context) error {
			jmxLogger.Flush()
			jmxLogger.close()
			return nil
		},
	})

	return Provides{
		Comp: jmxLogger,
	}, nil
}

func (j logger) JMXInfo(v ...interface{}) {
	j.inner.Info(v...)
}

func (j logger) JMXError(v ...interface{}) error {
	return j.inner.Error(v...)
}

func (j logger) Flush() {
	j.inner.Flush()
}

// close is used to ensure we release any resource associated with the JMXLogger
func (j logger) close() {
	j.inner.Close()
}
