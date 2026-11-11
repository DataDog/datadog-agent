// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package datadogexporter

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/plog"
)

type recordingLogs struct{ consumertest.LogsSink }

func (*recordingLogs) Start(context.Context, component.Host) error { return nil }
func (*recordingLogs) Shutdown(context.Context) error              { return nil }

type recordingDI struct {
	records int
	err     error
}

func (d *recordingDI) ConsumeDIEvents(_ context.Context, logs plog.Logs) error {
	d.records += logs.LogRecordCount()
	return d.err
}

func TestDIEventsNeverReachOrdinaryLogs(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		t.Run(map[bool]string{false: "enabled", true: "disabled"}[unavailable], func(t *testing.T) {
			input := plog.NewLogs()
			scope := input.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty()
			scope.LogRecords().AppendEmpty().Body().SetStr("ordinary application log")
			for _, name := range []string{"dd.debugger.snapshot", "dd.debugger.log", "dd.debugger.diagnostic", "dd.debugger.symbols", "dd.debugger.future"} {
				scope.LogRecords().AppendEmpty().SetEventName(name)
			}
			scope.LogRecords().AppendEmpty().Attributes().PutStr("datadog.di.payload", "{}")
			ordinary := &recordingLogs{}
			di := &recordingDI{}
			e := &diRoutingExporter{Logs: ordinary}
			if !unavailable {
				e.di = di
			}
			err := e.ConsumeLogs(t.Context(), input)
			if unavailable {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, 6, di.records)
			}
			require.Equal(t, 1, ordinary.LogRecordCount())
			require.Equal(t, 7, input.LogRecordCount())
		})
	}
}

func TestRejectedDIHasNoLogFallback(t *testing.T) {
	input := plog.NewLogs()
	scope := input.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty()
	scope.Scope().SetName("com.datadoghq.dynamic_instrumentation")
	scope.LogRecords().AppendEmpty().Body().SetStr("invalid DI envelope")
	ordinary := &recordingLogs{}
	e := &diRoutingExporter{Logs: ordinary, di: &recordingDI{err: errors.New("unsupported schema")}}
	require.Error(t, e.ConsumeLogs(t.Context(), input))
	require.Zero(t, ordinary.LogRecordCount())
}
