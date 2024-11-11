// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package traceroute

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"
)

var lookupAddrFn = net.DefaultResolver.LookupAddr

// GetReverseDNSForIP returns the reverse DNS for the given IP address as a net.IP.
func GetReverseDNSForIP(destIP net.IP) string {
	if destIP == nil {
		return ""
	}
	return GetHostname(destIP.String())
}

// GetHostname returns the hostname for the given IP address as a string.
func GetHostname(ipAddr string) string {
	currHost := ""
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	currHostList, err := lookupAddrFn(ctx, ipAddr)
	if errors.Is(err, context.Canceled) {
		tracerouteRunnerTelemetry.reverseDNSTimetouts.Inc()
	}

	if len(currHostList) > 0 {
		// TODO: Reverse DNS: Do we need to handle cases with multiple DNS being returned?
		currHost = currHostList[0]
	} else {
		currHost = ipAddr
	}
	// Trim trailing `.` in hostname since users are more familiar with this form.
	currHost = strings.TrimRight(currHost, ".")
	return currHost
}
