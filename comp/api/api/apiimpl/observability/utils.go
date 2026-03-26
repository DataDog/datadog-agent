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
	"github.com/gorilla/mux"
	"github.com/urfave/negroni"
)

// routeCaptureKey is the context key used to share route template capture between
// the outer telemetry middleware and the inner CaptureRouteTemplateMiddleware.
type routeCaptureKey struct{}

// routeCapture holds the matched route template filled in from inside gorilla/mux context.
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

// CaptureRouteTemplateMiddleware must be registered via gorilla/mux Router.Use() so that it runs
// inside the routing context where mux.CurrentRoute returns the matched route. It fills in the
// routeCapture that was planted in the request context by the outer telemetry middleware, allowing
// the outer middleware to read the route template after the request is handled.
//
// Without this middleware, requests that contain path variables (e.g. /{component}/status) would
// produce high-cardinality metric tags because each unique component value would become a tag.
func CaptureRouteTemplateMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if capture, ok := r.Context().Value(routeCaptureKey{}).(*routeCapture); ok {
			if route := mux.CurrentRoute(r); route != nil {
				if template, err := route.GetPathTemplate(); err == nil {
					capture.template = template
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// extractStatusCodeHandler is a middleware which extracts the status code from the response,
// and stores it in the provided pointer.
func extractStatusCodeHandler(status *int) mux.MiddlewareFunc {
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
func timeHandler(clock clock.Clock, duration *time.Duration) mux.MiddlewareFunc {
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
