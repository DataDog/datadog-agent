// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

// pingProbeTarget is the loopback address. It is always reachable, so a failed
// ping means this agent lacks the privileges to send ICMP at all.
const pingProbeTarget = "127.0.0.1"

// connectivityChecker runs ping and SNMP probes against a batch of addresses.
// It is satisfied by the networkdevices component reached over the agent IPC
// endpoint.
type connectivityChecker interface {
	CheckConnectivity(ctx context.Context, req connectivity.Request) (connectivity.Result, error)
}

// probePing reports whether this agent can send ICMP echo requests.
//
// The connectivity engine maps every ping failure to "unreachable", including
// a permissions failure. Without this probe, an agent that cannot ping would
// report every address in the org as unreachable. When the probe fails, ping
// is disabled for the whole component and ping_status is left empty instead.
func probePing(ctx context.Context, checker connectivityChecker) bool {
	res, err := checker.CheckConnectivity(ctx, connectivity.Request{
		Targets: []string{pingProbeTarget},
		Checks:  []string{connectivity.CheckPing},
		PingOptions: &connectivity.PingOptions{
			Count:      1,
			IntervalMs: defaultPingIntervalMs,
			TimeoutMs:  defaultPingTimeoutMs,
		},
		Workers: 1,
	})
	if err != nil {
		return false
	}

	for _, d := range res.Devices {
		if d.PingResult != nil && d.PingResult.Success {
			return true
		}
	}
	return false
}
