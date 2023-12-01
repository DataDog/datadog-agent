// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package daemonimpl

import (
	"net/http"
	"sync"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/serverless/daemon"
	"github.com/DataDog/datadog-agent/pkg/serverless/executioncontext"
	"github.com/DataDog/datadog-agent/pkg/serverless/flush"
	"github.com/DataDog/datadog-agent/pkg/serverless/invocationlifecycle"
	serverlessLog "github.com/DataDog/datadog-agent/pkg/serverless/logs"
	"github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule defines the fx options for the mock component.
var MockModule = fxutil.Component(
	fx.Provide(newMock),
)

func newMock(deps dependencies) daemon.Mock {
	mux := http.NewServeMux()

	daemon := &Daemon{
		httpServer:       &http.Server{Addr: deps.Params.Addr, Handler: mux},
		mux:              mux,
		RuntimeWg:        &sync.WaitGroup{},
		flushStrategy:    &flush.AtTheEnd{},
		ExtraTags:        &serverlessLog.Tags{},
		ExecutionContext: &executioncontext.ExecutionContext{},
		MetricAgent: &metrics.ServerlessMetricAgent{
			SketchesBucketOffset: deps.Params.SketchesBucketOffset,
		},
		TraceAgent:    &trace.ServerlessTraceAgent{},
		ShutdownDelay: 0,
	}

	mux.Handle("/lambda/flush", &Flush{daemon})
	mux.Handle("/lambda/start-invocation", wrapOtlpError(&StartInvocation{daemon}))
	mux.Handle("/lambda/end-invocation", wrapOtlpError(&EndInvocation{daemon}))
	mux.Handle("/trace-context", &TraceContext{daemon})

	return daemon
}

// GetInvocationProcessor gets the value og InvocationProcessor
func (d *Daemon) GetInvocationProcessor() invocationlifecycle.InvocationProcessor {
	return d.InvocationProcessor
}

// GetRuntimeWg gets the value of RuntimeWg
func (d *Daemon) GetRuntimeWg() *sync.WaitGroup {
	return d.RuntimeWg
}

// GetStopped gets the value of Stopped
func (d *Daemon) GetStopped() bool {
	return d.Stopped
}

// GetTellDaemonRuntimeDoneOnce gets the value of TellDaemonRuntimeDoneOnce
func (d *Daemon) GetTellDaemonRuntimeDoneOnce() *sync.Once {
	return d.TellDaemonRuntimeDoneOnce
}

// SetInvocationProcessor sets the value for InvocationProcessor
func (d *Daemon) SetInvocationProcessor(m invocationlifecycle.InvocationProcessor) {
	d.InvocationProcessor = m
}
