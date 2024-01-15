// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"net/http"
	"time"

	"github.com/urfave/negroni"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LogResponseHandler is a middleware that logs the response code and other various information about the request
func LogResponseHandler(serverName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			lrw := negroni.NewResponseWriter(w)
			start := time.Now()
			next.ServeHTTP(lrw, r)
			duration := time.Since(start)

			var logFunc func(format string, args ...interface{})
			code := lrw.Status()
			if code >= 100 && code < 400 {
				logFunc = log.Debugf
			} else if code >= 400 && code < 500 {
				logFunc = func(format string, args ...interface{}) { log.Warnf(format, args...) }
			} else { // >= 500 or < 100
				logFunc = func(format string, args ...interface{}) { log.Errorf(format, args...) }
			}

			format := "%s: %s %s from %s took %s and returned http code %d"
			logFunc(format, serverName, r.Method, r.RequestURI, r.RemoteAddr, duration, code)
		})
	}
}
