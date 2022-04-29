// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package utils

import (
	"net/http"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DefaultMaxConcurrentRequests determines the maximum number of requests in-flight for a given handler
// We choose 2 because one is for regular agent checks and another one is for manual troubleshooting
const DefaultMaxConcurrentRequests = 2

// WithConcurrencyLimit enforces a maximum number of concurrent requests over
// over a certain HTTP handler function
func WithConcurrencyLimit(limit int, original func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	var inFlight int64
	return func(w http.ResponseWriter, req *http.Request) {
		current := atomic.AddInt64(&inFlight, 1)
		defer atomic.AddInt64(&inFlight, -1)

		if current > int64(limit) {
			log.Warnf("rejecting request for path=%s concurrency_limit=%d", req.URL.Path, limit)
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		original(w, req)
	}
}
