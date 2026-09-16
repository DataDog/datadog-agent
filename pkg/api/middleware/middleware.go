// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package middleware provides common HTTP middleware for agent API servers.
package middleware

import (
	stdLog "log"
	"net/http"
	"runtime"
)

// RecoveryHandler returns a middleware that recovers from panics and logs them.
func RecoveryHandler(logger *stdLog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					const size = 64 << 10
					buf := make([]byte, size)
					buf = buf[:runtime.Stack(buf, false)]
					logger.Printf("panic serving %s: %v\n%s", r.URL, err, buf)
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// RequireContentType returns a middleware that rejects requests whose Content-Type header
// does not match the given value with 415 Unsupported Media Type.
func RequireContentType(contentType string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != contentType {
				http.Error(w, "Content-Type must be "+contentType, http.StatusUnsupportedMediaType)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
