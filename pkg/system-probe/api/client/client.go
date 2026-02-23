// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package client contains the client for the API exposed by system-probe
package client

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/funcs"
)

var (
	// ErrNotImplemented is an error used when system-probe is attempted to be accessed on an unsupported OS
	ErrNotImplemented = errors.New("system-probe unsupported")
	// ErrNotStartedYet is an error used when system-probe is attempted to be
	// accessed while it hasn't started yet (and could still be reasonably
	// expected to)
	ErrNotStartedYet = errors.New("system-probe not started yet")
	// ErrNotAvailable is an error used when system-probe failed to start
	// within the startup timeout and is considered permanently unavailable
	// for this agent lifetime. Checks should use IgnoreStartupError() to
	// suppress this error and avoid noisy error reporting in Fleet Automation.
	ErrNotAvailable = errors.New("system-probe not available")
)

// Get returns a http client configured to talk to the system-probe
var Get = funcs.MemoizeArgNoError[string, *http.Client](get)

const defaultHTTPTimeout = 10 * time.Second

func get(socketPath string) *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:          2,
			IdleConnTimeout:       idleConnTimeout,
			DialContext:           DialContextFunc(socketPath),
			TLSHandshakeTimeout:   1 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
			ExpectContinueTimeout: 50 * time.Millisecond,
		},
	}
}

func constructURL(module string, endpoint string) string {
	u, _ := url.Parse("http://sysprobe")
	if module != "" {
		u = u.JoinPath(module)
	}
	path, query, found := strings.Cut(endpoint, "?")
	u = u.JoinPath(path)
	if found {
		u.RawQuery = query
	}
	return u.String()
}

// URL constructs a system-probe URL for a module-less endpoint.
func URL(endpoint string) string {
	return constructURL("", endpoint)
}

// DebugURL constructs a system-probe URL for the debug module and endpoint.
func DebugURL(endpoint string) string {
	return constructURL("debug", endpoint)
}

// ModuleURL constructs a system-probe ModuleURL given the specified module and endpoint.
func ModuleURL(module types.ModuleName, endpoint string) string {
	return constructURL(string(module), endpoint)
}

// ReadAllResponseBody reads the entire HTTP response body as a byte slice
func ReadAllResponseBody(resp *http.Response) ([]byte, error) {
	// if we are not able to determine the content length
	// we read the whole body without pre-allocation
	if resp.ContentLength <= 0 {
		return io.ReadAll(resp.Body)
	}

	// if we know the content length we pre-allocate the buffer
	var buf bytes.Buffer
	buf.Grow(int(resp.ContentLength))

	_, err := buf.ReadFrom(resp.Body)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
