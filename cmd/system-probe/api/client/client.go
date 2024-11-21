// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package client contains the client for the API exposed by system-probe
package client

import (
	"errors"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/funcs"
)

var (
	// ErrNotImplemented is an error used when system-probe is attempted to be accessed on an unsupported OS
	ErrNotImplemented = errors.New("system-probe unsupported")
)

// Get returns a http client configured to talk to the system-probe
var Get = funcs.MemoizeArgNoError[string, *http.Client](get)

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
