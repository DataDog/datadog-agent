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

// logComponent wraps *pkglog.Wrapper and adds DrainErrorLogs, which requires
// converting between handlers.CapturedLog and logdef.CapturedLog. This conversion
// lives here because comp/core/log/impl already imports both packages; putting it
// in pkg/util/log would invert the dependency direction.
type logComponent struct {
	*pkglog.Wrapper
}

func (l *logComponent) DrainErrorLogs() []logdef.CapturedLog {
	raw := pkglog.DrainCapturedLogs()
	if len(raw) == 0 {
		return nil
	}
	out := make([]logdef.CapturedLog, len(raw))
	for i, cl := range raw {
		out[i] = logdef.CapturedLog{
			Level:     cl.Level,
			Message:   cl.Message,
			Timestamp: cl.Timestamp,
			Attrs:     cl.Attrs,
		}
	}
	return out
}

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
	return &logComponent{pkglog.NewWrapper(2)}
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

	l := &logComponent{pkglog.NewWrapper(2)}
	deps.Lc.Append(compdef.Hook{OnStop: func(context.Context) error {
		l.Flush()
		return nil
	}})

	return Provides{Comp: l}, nil
}
