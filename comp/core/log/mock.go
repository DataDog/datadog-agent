// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"context"
	"strings"
	"testing"

	"github.com/cihub/seelog"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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

func newMockLogger(t testing.TB, lc fx.Lifecycle) (Component, error) {
	// Build a logger that only logs to t.Log(..)
	iface, err := seelog.LoggerFromWriterWithMinLevelAndFormat(&tbWriter{t}, seelog.TraceLvl,
		"%Date(2006-01-02 15:04:05 MST) | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %ExtraTextContext%Msg%n")
	if err != nil {
		return nil, err
	}

	// flush the seelog logger when the test app stops
	lc.Append(fx.Hook{OnStop: func(context.Context) error {
		iface.Flush()
		return nil
	}})

	// install the logger into pkg/util/log
	log.ChangeLogLevel(iface, "off")

	return &logger{}, nil
}
