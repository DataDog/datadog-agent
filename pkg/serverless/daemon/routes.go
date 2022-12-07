// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package daemon

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/serverless/invocationlifecycle"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const localTestEnvVar = "DD_LOCAL_TEST"

// Hello is a route called by the Datadog Lambda Library when it starts.
// It is used to detect the Datadog Lambda Library in the environment.
type Hello struct {
	daemon *Daemon
}

func (h *Hello) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debug("Hit on the serverless.Hello route.")
	h.daemon.LambdaLibraryDetected = true
}

// Flush is a route called by the Datadog Lambda Library when the runtime is done handling an invocation.
// It is no longer used, but the route is maintained for backwards compatibility.
type Flush struct {
	daemon *Daemon
}

func (f *Flush) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debug("Hit on the serverless.Flush route.")
	if len(os.Getenv(localTestEnvVar)) > 0 {
		// used only for testing purpose as the Logs API is not supported by the Lambda Emulator
		// thus we canot get the REPORT log line telling that the invocation is finished
		f.daemon.HandleRuntimeDone()
	}
}

// StartInvocation is a route that can be called at the beginning of an invocation to enable
// the invocation lifecyle feature without the use of the proxy.
type StartInvocation struct {
	daemon *Daemon
}

func (s *StartInvocation) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debug("Hit on the serverless.StartInvocation route.")
	startTime := time.Now()
	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		log.Error("Could not read StartInvocation request body")
		http.Error(w, "Could not read StartInvocation request body", 400)
		return
	}
	lambdaInvokeContext := invocationlifecycle.LambdaInvokeEventHeaders{
		TraceID:          r.Header.Get(invocationlifecycle.TraceIDHeader),
		ParentID:         r.Header.Get(invocationlifecycle.ParentIDHeader),
		SamplingPriority: r.Header.Get(invocationlifecycle.SamplingPriorityHeader),
	}
	startDetails := &invocationlifecycle.InvocationStartDetails{
		StartTime:             startTime,
		InvokeEventRawPayload: reqBody,
		InvokeEventHeaders:    lambdaInvokeContext,
		InvokedFunctionARN:    s.daemon.ExecutionContext.GetCurrentState().ARN,
	}

	s.daemon.InvocationProcessor.OnInvokeStart(startDetails)

	if s.daemon.InvocationProcessor.GetExecutionInfo().TraceID == 0 {
		log.Debug("no context has been found, the tracer will be responsible for initializing the context")
	} else {
		log.Debug("a context has been found, sending the context to the tracer")
		w.Header().Set(invocationlifecycle.TraceIDHeader, fmt.Sprintf("%v", s.daemon.InvocationProcessor.GetExecutionInfo().TraceID))
		w.Header().Set(invocationlifecycle.SamplingPriorityHeader, fmt.Sprintf("%v", s.daemon.InvocationProcessor.GetExecutionInfo().SamplingPriority))
	}
}

// EndInvocation is a route that can be called at the end of an invocation to enable
// the invocation lifecycle feature without the use of the proxy.
type EndInvocation struct {
	daemon *Daemon
}

func (e *EndInvocation) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debug("Hit on the serverless.EndInvocation route.")
	endTime := time.Now()
	ecs := e.daemon.ExecutionContext.GetCurrentState()
	responseBody, err := io.ReadAll(r.Body)
	if err != nil {
		err := log.Error("Could not read EndInvocation request body")
		http.Error(w, err.Error(), 400)
		return
	}
	var endDetails = invocationlifecycle.InvocationEndDetails{
		EndTime:            endTime,
		IsError:            r.Header.Get(invocationlifecycle.InvocationErrorHeader) == "true",
		RequestID:          ecs.LastRequestID,
		ResponseRawPayload: responseBody,
		ColdStartDuration:  ecs.ColdstartDuration,
	}
	executionContext := e.daemon.InvocationProcessor.GetExecutionInfo()
	if executionContext.TraceID == 0 {
		log.Debug("no context has been found yet, injecting it now via headers from the tracer")
		invocationlifecycle.InjectContext(executionContext, r.Header)
	}
	invocationlifecycle.InjectSpanID(executionContext, r.Header)
	e.daemon.InvocationProcessor.OnInvokeEnd(&endDetails)
}

// TraceContext is a route called by tracer so it can retrieve the tracing context
type TraceContext struct {
	daemon *Daemon
}

func (tc *TraceContext) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	executionInfo := tc.daemon.InvocationProcessor.GetExecutionInfo()
	log.Debug("Hit on the serverless.TraceContext route.")
	w.Header().Set(invocationlifecycle.TraceIDHeader, fmt.Sprintf("%v", executionInfo.TraceID))
	w.Header().Set(invocationlifecycle.SpanIDHeader, fmt.Sprintf("%v", executionInfo.SpanID))
	w.Header().Set(invocationlifecycle.SamplingPriorityHeader, fmt.Sprintf("%v", executionInfo.SamplingPriority))
}
