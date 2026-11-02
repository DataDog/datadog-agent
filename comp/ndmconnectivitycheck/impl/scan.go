// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmconnectivitycheckimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/pingprobe"
	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

// scan probes one request's targets.
func (c *connectivityCheck) scan(ctx context.Context, req connectivity.Request) (connectivity.Result, error) {
	opts, err := toProbeOptions(req, c.pingCapability())
	if err != nil {
		return connectivity.Result{}, err
	}

	results, err := probe.Scan(ctx, workers(req.Workers), req.Targets, opts)
	if err != nil {
		return connectivity.Result{}, err
	}
	return toConnectivityResult(results), nil
}

// workers is the request's concurrency, which bounds one check. The number of
// checks in flight is bounded by the Private Action Runner's task concurrency.
func workers(hint int) int {
	if hint < 1 {
		return 1
	}
	return hint
}

// pingCapability detects whether this agent can ping, once.
func (c *connectivityCheck) pingCapability() pingprobe.Capability {
	c.pingOnce.Do(func() {
		c.ping = pingprobe.Detect()
		if !c.ping.Available {
			c.logger.Warnf("ndmconnectivitycheck: the ping probe is not available on this agent: %s", c.ping.Reason)
		}
	})
	return c.ping
}
