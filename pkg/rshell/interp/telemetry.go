// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"context"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

const maxTelemetrySpans = 10_000

var (
	// The dependency tracer is process-global, so the cap must be too. This
	// counter is never reset because rshell cannot observe when its queue flushes.
	telemetrySpansStarted atomic.Uint64
	droppedTelemetrySpan  rshellTelemetrySpan = noOpTelemetrySpan{}
)

type rshellTelemetrySpan interface {
	Finish(error)
	SetResourceName(string)
	SetTag(string, any)
}

type noOpTelemetrySpan struct{}

func (noOpTelemetrySpan) Finish(error)           {}
func (noOpTelemetrySpan) SetResourceName(string) {}
func (noOpTelemetrySpan) SetTag(string, any)     {}

func startTelemetrySpan(ctx context.Context, operationName string) (rshellTelemetrySpan, context.Context) {
	for {
		started := telemetrySpansStarted.Load()
		if started >= maxTelemetrySpans {
			return droppedTelemetrySpan, ctx
		}
		if telemetrySpansStarted.CompareAndSwap(started, started+1) {
			return telemetry.StartSpanFromContext(ctx, operationName)
		}
	}
}
