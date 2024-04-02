// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package logimpl

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/cihub/seelog"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMockLogger),
	)
}

// TraceMockModule defines the fx options for the mock component in its Trace variant.
func TraceMockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newTraceMockLogger),
	)
}

// tbWriter is an implementation of io.Writer that sends lines to
// testing.TB#Log.
type tbWriter struct {
	t testing.TB
}

// Write implements Writer#Write.
func (tbw *tbWriter) Write(p []byte) (n int, err error) {
	// this assumes that seelog always writes one log entry in one Write call
	msg := strings.TrimSuffix(string(p), "\n")
	tbw.t.Log(msg)
	return len(p), nil
}

func newMockLogger(t testing.TB, lc fx.Lifecycle) (log.Component, error) {
	// Build a logger that only logs to t.Log(..)
	iface, err := seelog.LoggerFromWriterWithMinLevelAndFormat(&tbWriter{t}, seelog.TraceLvl,
		"%Date(2006-01-02 15:04:05 MST) | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %ExtraTextContext%Msg%n")
	if err != nil {
		return nil, err
	}

	// flush the seelog logger when the test app stops
	lc.Append(fx.Hook{OnStop: func(context.Context) error {
		// stop using the logger to avoid a race condition
		pkglog.ChangeLogLevel(seelog.Default, "debug")
		iface.Flush()
		return nil
	}})

	// install the logger into pkg/util/log
	pkglog.ChangeLogLevel(iface, "debug")

	return &logger{}, nil
}

func newTraceMockLogger(t testing.TB, lc fx.Lifecycle, params Params, cfg config.Component) (log.Component, error) {
	// Make sure we are setting a default value on purpose.
	logFilePath := params.logFileFn(cfg)
	if logFilePath != os.Getenv("DDTEST_DEFAULT_LOG_FILE_PATH") {
		return nil, fmt.Errorf("unexpected default log file path: %q", logFilePath)
	}
	return newMockLogger(t, lc)
}
