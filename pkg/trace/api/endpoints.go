package api

import "net/http"

type endpoint struct {
	Name    string
	Handler func(*HTTPReceiver) http.Handler
	Hidden  bool
}

var endpoints = []endpoint{
	{
		Name:    "/spans",
		Handler: func(r *HTTPReceiver) http.Handler { return r.handleWithVersion(v01, r.handleTraces) },
		Hidden:  true,
	},
	{
		Name:    "/services",
		Handler: func(r *HTTPReceiver) http.Handler { return r.handleWithVersion(v01, r.handleServices) },
		Hidden:  true,
	},
	{
		Name:    "/v0.1/spans",
		Handler: func(r *HTTPReceiver) http.Handler { return r.handleWithVersion(v01, r.handleTraces) },
		Hidden:  true,
	},
	{
		Name:    "/v0.1/services",
		Handler: func(r *HTTPReceiver) http.Handler { return r.handleWithVersion(v01, r.handleServices) },
		Hidden:  true,
	},
	{
		Name:    "/v0.2/traces",
		Handler: func(r *HTTPReceiver) http.Handler { return r.handleWithVersion(v02, r.handleTraces) },
		Hidden:  true,
	},
	{
		Name:    "/v0.2/services",
		Handler: func(r *HTTPReceiver) http.Handler { return r.handleWithVersion(v02, r.handleServices) },
		Hidden:  true,
	},
	{
		Name:    "/v0.3/traces",
		Handler: func(r *HTTPReceiver) http.Handler { return r.handleWithVersion(v03, r.handleTraces) },
	},
	{
		Name:    "/v0.3/services",
		Handler: func(r *HTTPReceiver) http.Handler { return r.handleWithVersion(v03, r.handleServices) },
	},
	{
		Name:    "/v0.4/traces",
		Handler: func(r *HTTPReceiver) http.Handler { return r.handleWithVersion(v04, r.handleTraces) },
	},
	{
		Name:    "/v0.4/services",
		Handler: func(r *HTTPReceiver) http.Handler { return r.handleWithVersion(v04, r.handleServices) },
	},
	{
		Name:    "/v0.5/traces",
		Handler: func(r *HTTPReceiver) http.Handler { return r.handleWithVersion(v05, r.handleTraces) },
	},
	{
		Name:    "/v0.5/stats",
		Handler: func(r *HTTPReceiver) http.Handler { return http.HandlerFunc(r.handleStats) },
	},
	{
		Name:    "/profiling/v1/input",
		Handler: func(r *HTTPReceiver) http.Handler { return r.profileProxyHandler() },
	},
}
