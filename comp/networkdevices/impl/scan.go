// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package networkdevicesimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/pingprobe"
	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

// scan probes one request's targets. Its error is either an invalid request or
// a cancelled context.
func (c *networkDevicesImpl) scan(ctx context.Context, req connectivity.Request) (connectivity.Result, error) {
	opts, err := toProbeOptions(req, c.pingCapability())
	if err != nil {
		return connectivity.Result{}, err
	}

	workers := c.workers(req.Workers)
	if err := c.sem.Acquire(ctx, int64(workers)); err != nil {
		return connectivity.Result{}, err
	}
	defer c.sem.Release(int64(workers))

	results, err := probe.Scan(ctx, workers, req.Targets, opts)
	if err != nil {
		return connectivity.Result{}, err
	}
	return toConnectivityResult(results), nil
}

// workers keeps the request's hint inside this agent's interactive budget.
func (c *networkDevicesImpl) workers(hint int) int {
	if hint < 1 {
		return 1
	}
	if hint > c.maxWorkers {
		return c.maxWorkers
	}
	return hint
}

// pingCapability detects whether this agent can ping, once.
func (c *networkDevicesImpl) pingCapability() pingprobe.Capability {
	c.pingOnce.Do(func() {
		c.ping = pingprobe.Detect()
		if !c.ping.Available {
			c.logger.Warnf("networkdevices: the ping probe is not available on this agent: %s", c.ping.Reason)
		}
	})
	return c.ping
}
