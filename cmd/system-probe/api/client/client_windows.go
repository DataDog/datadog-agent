// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package client

import (
	"net/http"

	processNet "github.com/DataDog/datadog-agent/pkg/process/net"
)

// newSystemProbeClient returns a http client configured to talk to the system-probe
// This is a simple wrapper around process_net.NewSystemProbeHttpClient because
// Linux is unable to import pkg/process/net due to size restrictions.
func newSystemProbeClient(_ string) *http.Client {
	return processNet.NewSystemProbeClient()
}
