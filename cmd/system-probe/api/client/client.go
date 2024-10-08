// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package client contains the client for the API exposed by system-probe
package client

import (
	"net/http"
)

// Get returns a http client configured to talk to the system-probe
func Get(socketPath string) *http.Client {
	return newSystemProbeClient(socketPath)
}
