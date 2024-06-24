// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observability implements various observability handlers for the API servers
package observability

import (
	"net/http"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type logFunc func(format string, args ...interface{})

const logFormat = "%s: %s %s from %s to %s took %s and returned http code %d"

func getLogFunc(code int) logFunc {
	if code >= 100 && code < 400 {
		return log.Debugf
	}
	if code >= 400 && code < 500 {
		return func(format string, args ...interface{}) { log.Warnf(format, args...) }
	}
	// >= 500 or < 100
	return func(format string, args ...interface{}) { log.Errorf(format, args...) }
}

// LogResponseHandler is a middleware that logs the response code and other various information about the request
func LogResponseHandler(servername string) mux.MiddlewareFunc {
	return logResponseHandler(servername, getLogFunc, clock.New())
}

// logResponseHandler takes getLogFunc as a parameter to allow for testing
func logResponseHandler(serverName string, getLogFunc func(int) logFunc, clock clock.Clock) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var code int
			next = extractStatusHandler(&code)(next)

			var duration time.Duration
			next = timeHandler(clock, &duration)(next)

			next.ServeHTTP(w, r)

			logFunc := getLogFunc(code)
			path := extractPath(r)
			logFunc(logFormat, serverName, r.Method, path, r.RemoteAddr, r.Host, duration, code)
		})
	}
}
