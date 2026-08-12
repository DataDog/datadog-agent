// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package jmxloggerimpl implements the logger for JMX.
package jmxloggerimpl

import (
	"context"
	"fmt"

	compdef "github.com/DataDog/datadog-agent/comp/def"

	jmxlogger "github.com/DataDog/datadog-agent/comp/agent/jmxlogger/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
)

// Requires defines the dependencies for the jmxlogger component.
type Requires struct {
	Lc     compdef.Lifecycle
	Config config.Component
	Params jmxlogger.Params
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
	config := reqs.Config
	var inner jmxLoggerInterface
	var err error

	if reqs.Params.IsFromCLI() {
		inner, err = pkglogsetup.BuildJMXLogger(reqs.Params.GetLogFile(), "", false, true, false, config)
		if err != nil {
			return Provides{}, fmt.Errorf("Unable to set up JMX logger: %v", err)
		}
	} else {
		syslogURI := pkglogsetup.GetSyslogURI(config)
		jmxLogFile := config.GetString("jmx_log_file")

		if config.GetBool("disable_file_logging") {
			// this will prevent any logging on file
			jmxLogFile = ""
		}

		inner, err = pkglogsetup.BuildJMXLogger(
			jmxLogFile,
			syslogURI,
			config.GetBool("syslog_rfc"),
			config.GetBool("log_to_console"),
			config.GetBool("log_format_json"),
			config,
		)

		if err != nil {
			return Provides{}, fmt.Errorf("Error while setting up logging, exiting: %v", err)
		}
	}

	jmxLog := logger{
		inner: inner,
	}

	reqs.Lc.Append(compdef.Hook{
		OnStop: func(_ context.Context) error {
			jmxLog.Flush()
			jmxLog.close()
			return nil
		},
	})

	return Provides{Comp: jmxLog}, nil
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
