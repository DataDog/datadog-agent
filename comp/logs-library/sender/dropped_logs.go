// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"net"
	"net/url"

	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
)

// workerDropTargets names each destination for the dropped-logs health issue, in
// the order the worker iterates them. Destinations are built in
// GetReliableEndpoints order, which starts with the main endpoint.
func workerDropTargets(destinations []client.Destination, reliable bool) []metrics.DropTarget {
	targets := make([]metrics.DropTarget, len(destinations))
	for i, destination := range destinations {
		kind := metrics.DropTargetBestEffort
		switch {
		case destination.IsMRF():
			kind = metrics.DropTargetMRF
		case reliable && i == 0:
			kind = metrics.DropTargetPrimary
		case reliable:
			kind = metrics.DropTargetAdditional
		}
		targets[i] = metrics.DropTarget{Host: targetHost(destination.Target()), Kind: kind}
	}
	return targets
}

// targetHost reduces a destination target to its host: HTTP targets are URLs and
// TCP targets are host:port.
func targetHost(target string) string {
	if u, err := url.Parse(target); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	if host, _, err := net.SplitHostPort(target); err == nil {
		return host
	}
	return target
}
