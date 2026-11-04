// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package runners

import (
	"context"
	"strconv"
	"strings"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/devtracing"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/observability"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// DEVELOPMENT-ONLY — Remote Queries POC. Delete this file, its call site in
// workflow_task_executor.go, and the devtracing package with the POC.
//
// The runner's action.run span is emitted by the Fleet mini-tracer to
// instrumentation telemetry only, so it never reaches the development
// trace-agent and the remote_queries.agent_execute span would sit under a
// missing parent in ddstaging. The mirror plugs that hole: a normal
// dd-trace-go span with the same span ID, trace, and parent as the mini-tracer
// span, sent through the development trace-agent. It is observability only and
// fails open — any problem yields no mirror and execution is unchanged.

// remoteQueriesFQNPrefix selects the remote-queries actions, the only ones
// mirrored.
const remoteQueriesFQNPrefix = "com.datadoghq.remoteaction.queries."

const (
	miniTracerTraceIDEnv          = "DATADOG_TRACE_ID"
	miniTracerParentIDEnv         = "DATADOG_PARENT_ID"
	miniTracerSamplingPriorityEnv = "DATADOG_SAMPLING_PRIORITY"

	// miniTracerDefaultSamplingPriority is the mini-tracer flush default (user
	// keep) for a trace that propagated no priority.
	miniTracerDefaultSamplingPriority = 2
)

// startDevelopmentActionRunMirror starts the mirror of the mini-tracer's
// action.run span active on ctx (the context returned by
// telemetry.StartSpanFromUint64IDs) and returns a function finishing it with
// the action's error. It never changes ctx. It returns a no-op when
// development tracing is disabled, the action is not a remote-queries action,
// the task carries no trace ID, or the mini-tracer span's identity cannot be
// read.
func startDevelopmentActionRunMirror(ctx context.Context, task *types.Task, fqn string) (finish func(err error)) {
	noop := func(error) {}
	if !devtracing.Enabled() || !strings.HasPrefix(fqn, remoteQueriesFQNPrefix) {
		return noop
	}
	if task == nil || task.Data.Attributes == nil || task.Data.Attributes.TraceId == 0 {
		return noop
	}
	taskTraceID, taskSpanID := task.Data.Attributes.TraceId, task.Data.Attributes.SpanId

	var traceID, ownSpanID uint64
	samplingPriority := miniTracerDefaultSamplingPriority
	for _, entry := range telemetry.EnvFromContext(ctx) {
		key, value, _ := strings.Cut(entry, "=")
		var err error
		switch key {
		case miniTracerTraceIDEnv:
			traceID, err = strconv.ParseUint(value, 10, 64)
		case miniTracerParentIDEnv:
			ownSpanID, err = strconv.ParseUint(value, 10, 64)
		case miniTracerSamplingPriorityEnv:
			samplingPriority, err = strconv.Atoi(value)
		}
		if err != nil {
			return noop
		}
	}
	if ownSpanID == 0 || traceID != taskTraceID {
		return noop
	}

	devtracing.EnsureTracer()
	// The propagator extracts the task's trace, parent, and sampling priority;
	// the mirror is started on a detached context so it never touches ctx.
	span, _ := tracer.StartSpanFromPropagatedContext(
		context.Background(),
		observability.ActionRunOperation,
		tracer.TextMapCarrier{
			tracer.DefaultTraceIDHeader:  strconv.FormatUint(taskTraceID, 10),
			tracer.DefaultParentIDHeader: strconv.FormatUint(taskSpanID, 10),
			tracer.DefaultPriorityHeader: strconv.Itoa(samplingPriority),
		},
		tracer.WithSpanID(ownSpanID),
		tracer.ServiceName(observability.ParService),
		tracer.ResourceName(fqn),
		tracer.Tag("task_id", task.Data.ID),
	)
	if span == nil {
		return noop
	}
	return func(err error) {
		if err != nil {
			span.Finish(tracer.WithError(err))
			return
		}
		span.Finish()
	}
}
