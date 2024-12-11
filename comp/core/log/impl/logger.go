// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package logimpl implements a component to handle logging internal to the agent.
package logimpl

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logdef "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
)

// NewTemporaryLoggerWithoutInit returns a logger component instance. It assumes the logger has already been
// initialized beforehand.
//
// This function should be used when all these conditions are true:
// - You write or update code which uses a lot of logging.
// - You want the code to be components ready.
// - logger.Component cannot be injected.
//
// It should not be used when:
// - You add few logging functions.
// - When the instance of logger.Component is reachable in less than 5 stack frames.
// - It doesn't make the migration to log.Component easier.
func NewTemporaryLoggerWithoutInit() logdef.Component {
	return pkglog.NewWrapper(2)
}

// Requires declares the input types to the logger component constructor
type Requires struct {
	Lc     compdef.Lifecycle
	Params logdef.Params
	Config config.Component
}

// Provides defines the output of the log component
type Provides struct {
	Comp logdef.Component
}

// NewComponent creates a log.Component using the provided config
func NewComponent(deps Requires) (Provides, error) {
	if !deps.Params.IsLogLevelFnSet() {
		return Provides{}, errors.New("must call one of core.BundleParams.ForOneShot or ForDaemon")
	}

	err := pkglogsetup.SetupLogger(
		pkglogsetup.LoggerName(deps.Params.LoggerName()),
		deps.Params.LogLevelFn(deps.Config),
		deps.Params.LogFileFn(deps.Config),
		deps.Params.LogSyslogURIFn(deps.Config),
		deps.Params.LogSyslogRFCFn(deps.Config),
		deps.Params.LogToConsoleFn(deps.Config),
		deps.Params.LogFormatJSONFn(deps.Config),
		deps.Config)
	if err != nil {
		return Provides{}, err
	}

	l := pkglog.NewWrapper(2)
	deps.Lc.Append(compdef.Hook{OnStop: func(context.Context) error {
		l.Flush()
		return nil
	}})

	return Provides{Comp: l}, nil
}
