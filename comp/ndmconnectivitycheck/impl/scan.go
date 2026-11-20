// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmconnectivitycheckimpl

import (
	"context"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe"
	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

// scan probes one request's targets. With target groups, each group's targets
// are tried only with the credentials the group names.
func (c *connectivityCheck) scan(ctx context.Context, req connectivity.Request) (connectivity.Result, error) {
	if len(req.TargetGroups) == 0 {
		return c.scanTargets(ctx, req)
	}

	res := connectivity.Result{Devices: []connectivity.DeviceResult{}}
	for _, g := range req.TargetGroups {
		groupReq := req
		groupReq.Targets = g.TargetIPs
		groupReq.Credentials = credsByID(req.Credentials, g.CredentialIDs)
		groupRes, err := c.scanTargets(ctx, groupReq)
		if err != nil {
			return connectivity.Result{}, err
		}
		res.Devices = append(res.Devices, groupRes.Devices...)
	}
	return res, nil
}

// credsByID returns the credentials whose id is in ids.
func credsByID(creds []connectivity.SNMPCredential, ids []string) []connectivity.SNMPCredential {
	var out []connectivity.SNMPCredential
	for _, c := range creds {
		if slices.Contains(ids, c.ID) {
			out = append(out, c)
		}
	}
	return out
}

// scanTargets probes the request's targets with all its credentials.
func (c *connectivityCheck) scanTargets(ctx context.Context, req connectivity.Request) (connectivity.Result, error) {
	opts, err := toProbeOptions(req)
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
