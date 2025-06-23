// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && linux

// Package debugger provides utilities for testing the HTTP protocol.
package debugger

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/network/tracer"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetHTTPDebugEndpoint returns a handler for debugging HTTP requests.
func GetHTTPDebugEndpoint(tracer *tracer.Tracer) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		mon := tracer.USMMonitor()
		if mon == nil {
			log.Error("unable to retrieve USM monitor")
			w.WriteHeader(500)
			return
		}

		pidStr := r.URL.Query().Get("pid")
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			log.Errorf("invalid pid %q: %s", pidStr, err)
			w.WriteHeader(400)
			return
		}
		method := strings.ToUpper(r.URL.Query().Get("method")) + " "

		buf := [24]byte{}
		copy(buf[:len(method)], method)
		url := r.URL.Query().Get("url")
		copy(buf[len(method):], url)
		if err := mon.DebugHTTPPath(uint32(pid), buf, uint8(len(method)+len(url))); err != nil {
			log.Errorf("unable to debug HTTP path for pid %d: %s", pid, err)
			w.WriteHeader(500)
			return
		}
	}
}
