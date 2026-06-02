// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observability

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/urfave/negroni/v3"
)

// routeCaptureKey is the context key used to share the route template between
// the outer telemetry middleware and the per-handler wrapper.
type routeCaptureKey struct{}

// routeCapture holds the matched route template.
type routeCapture struct {
	template string
}

// extractPath extracts the original request path from the request.
// using r.URL.Path is not correct because when using http.StripPrefix it contains the stripped path
func extractPath(r *http.Request) string {
	reqURL, err := url.ParseRequestURI(r.RequestURI)
	if err != nil {
		return "<invalid url>" // redacted in case it contained sensitive information
	}
	return reqURL.Path
}

// WrapWithRouteTemplate registers h on mux for the given method and path, storing the
// path template in the capture context so the telemetry middleware can use it for metric
// cardinality reduction instead of the raw request path.
// When the mux is mounted under a path prefix, use MountWithPrefix at the mount site
// to have the prefix prepended automatically to the captured template.
func WrapWithRouteTemplate(mux *http.ServeMux, method, path string, h http.Handler) {
	mux.Handle(method+" "+path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if capture, ok := r.Context().Value(routeCaptureKey{}).(*routeCapture); ok {
			capture.template = path
		}
		h.ServeHTTP(w, r)
	}))
}

// MountWithPrefix strips prefix from the request path and passes it to h, then
// prepends prefix to whatever route template h stored in the capture context.
// Use this instead of http.StripPrefix when mounting a prefix-agnostic sub-mux:
// the sub-mux registers routes relative to its own root and the mount site owns
// the prefix, keeping the two decoupled.
func MountWithPrefix(prefix string, h http.Handler) http.Handler {
	stripped := http.StripPrefix(prefix, h)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stripped.ServeHTTP(w, r)
		if capture, ok := r.Context().Value(routeCaptureKey{}).(*routeCapture); ok && capture.template != "" {
			capture.template = prefix + capture.template
		}
	})
}

// extractStatusCodeHandler is a middleware which extracts the status code from the response,
// and stores it in the provided pointer.
func extractStatusCodeHandler(status *int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			lrw := negroni.NewResponseWriter(w)
			next.ServeHTTP(lrw, r)
			*status = lrw.Status()
		})
	}
}

// timeHandler is a middleware which measures the duration of the request,
// and stores it in the provided pointer.
func timeHandler(clock clock.Clock, duration *time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := clock.Now()
			next.ServeHTTP(w, r)
			*duration = clock.Since(start)
		})
	}
}

// withRouteCapture adds a routeCapture to the request context and returns
// the updated request and a pointer to the capture struct.
func withRouteCapture(r *http.Request) (*http.Request, *routeCapture) {
	capture := &routeCapture{}
	return r.WithContext(context.WithValue(r.Context(), routeCaptureKey{}, capture)), capture
}
