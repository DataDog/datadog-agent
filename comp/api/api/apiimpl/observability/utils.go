// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observability

import (
	"net/http"
	"net/url"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/gorilla/mux"
	"github.com/urfave/negroni"
)

// extractPath extracts the original request path from the request
// using r.URL.Path is not correct because when using http.StripPrefix it contains the stripped path
func extractPath(r *http.Request) string {
	reqURL, err := url.ParseRequestURI(r.RequestURI)
	if err != nil {
		return "<invalid url>" // redacted in case it contained sensitive information
	}
	return reqURL.Path
}

// extractStatusHandler is a middleware which extracts the status code from the response,
// and stores it in the provided pointer.
func extractStatusHandler(status *int) mux.MiddlewareFunc {
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
