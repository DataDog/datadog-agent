// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package daemon

import (
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Hello is a route called by the Datadog Lambda Library when it starts.
// It is used to detect the Datadog Lambda Library in the environment.
type Hello struct {
	daemon *Daemon
}

//nolint:revive // TODO(SERV) Fix revive linter
func (h *Hello) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debug("Hit on the serverless.Hello route.")
	h.daemon.LambdaLibraryDetected = true
}

// Flush is a route called by the Datadog Lambda Library when the runtime is done handling an invocation.
// It is no longer used, but the route is maintained for backwards compatibility.
type Flush struct {
	daemon *Daemon
}

//nolint:revive // TODO(SERV) Fix revive linter
func (f *Flush) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	panic("not called")
}

// StartInvocation is a route that can be called at the beginning of an invocation to enable
// the invocation lifecyle feature without the use of the proxy.
type StartInvocation struct {
	daemon *Daemon
}

func (s *StartInvocation) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	panic("not called")
}

// EndInvocation is a route that can be called at the end of an invocation to enable
// the invocation lifecycle feature without the use of the proxy.
type EndInvocation struct {
	daemon *Daemon
}

func (e *EndInvocation) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	panic("not called")
}

// TraceContext is a route called by tracer so it can retrieve the tracing context
type TraceContext struct {
	daemon *Daemon
}

//nolint:revive // TODO(SERV) Fix revive linter
func (tc *TraceContext) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	panic("not called")
}
