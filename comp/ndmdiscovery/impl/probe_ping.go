// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

const kindPing = "ping"

// pingProbeTarget is the loopback address. It is always reachable, so a failed
// ping means this agent cannot send ICMP at all.
const pingProbeTarget = "127.0.0.1"

// Defaults and bounds of the ping probe options. An out-of-bounds value falls
// back to its default.
const (
	defaultPingCount      = 1
	defaultPingIntervalMs = 1000
	defaultPingTimeoutMs  = 1000
	maxPingCount          = 10
	maxPingIntervalMs     = 60_000
	maxPingTimeoutMs      = 60_000
)

// pingOptions is the ping block of a range's probes object.
type pingOptions struct {
	Count      int `json:"count"`
	IntervalMs int `json:"interval_ms"`
	TimeoutMs  int `json:"timeout_ms"`
}

var _ probe = (*pingProbe)(nil)

// pingProbe scans a range with ICMP echo requests.
type pingProbe struct {
	checker connectivityChecker
	log     log.Component
	capable atomic.Bool
}

// newPingProbe builds the ping probe. It is unavailable until detect runs.
func newPingProbe(checker connectivityChecker, logger log.Component) *pingProbe {
	return &pingProbe{checker: checker, log: logger}
}

func (p *pingProbe) kind() string { return kindPing }

// detect pings the loopback address once, because the connectivity engine
// reports a missing privilege as an unreachable address.
func (p *pingProbe) detect(ctx context.Context) {
	p.capable.Store(probePing(ctx, p.checker))
}

func (p *pingProbe) available() bool { return p.capable.Load() }

func (p *pingProbe) parse(raw json.RawMessage) (probeConfig, error) {
	var opts pingOptions
	if err := json.Unmarshal(raw, &opts); err != nil {
		return nil, fmt.Errorf("the ping options are not an object: %w", err)
	}

	resolved := connectivity.PingOptions{
		Count:      defaultPingCount,
		IntervalMs: defaultPingIntervalMs,
		TimeoutMs:  defaultPingTimeoutMs,
	}
	if opts.Count != 0 {
		if opts.Count >= 1 && opts.Count <= maxPingCount {
			resolved.Count = opts.Count
		} else {
			p.log.Warnf("ndmdiscovery: ping count %d is out of range (expected 1-%d), using %d", opts.Count, maxPingCount, defaultPingCount)
		}
	}
	if opts.IntervalMs != 0 {
		if opts.IntervalMs >= 1 && opts.IntervalMs <= maxPingIntervalMs {
			resolved.IntervalMs = opts.IntervalMs
		} else {
			p.log.Warnf("ndmdiscovery: ping interval_ms %d is out of range (expected 1-%d), using %d", opts.IntervalMs, maxPingIntervalMs, defaultPingIntervalMs)
		}
	}
	if opts.TimeoutMs != 0 {
		if opts.TimeoutMs >= 1 && opts.TimeoutMs <= maxPingTimeoutMs {
			resolved.TimeoutMs = opts.TimeoutMs
		} else {
			p.log.Warnf("ndmdiscovery: ping timeout_ms %d is out of range (expected 1-%d), using %d", opts.TimeoutMs, maxPingTimeoutMs, defaultPingTimeoutMs)
		}
	}

	return &pingConfig{options: resolved}, nil
}

var _ probeConfig = (*pingConfig)(nil)

// pingConfig is the ping probe's configuration for one range.
type pingConfig struct {
	options connectivity.PingOptions
}

func (c *pingConfig) kind() string { return kindPing }

func (c *pingConfig) prepare() (probeRun, error) {
	return &pingRun{options: c.options}, nil
}

var _ probeRun = (*pingRun)(nil)

// pingRun is the ping probe's contribution to one cycle.
type pingRun struct {
	options connectivity.PingOptions
}

func (r *pingRun) fingerprint() string {
	return fmt.Sprintf("%s:count=%d interval=%d timeout=%d", kindPing, r.options.Count, r.options.IntervalMs, r.options.TimeoutMs)
}

func (r *pingRun) apply(req *connectivity.Request) {
	req.Checks = append(req.Checks, connectivity.CheckPing)
	options := r.options
	req.PingOptions = &options
}

func (r *pingRun) read(d connectivity.DeviceResult) *probeReading {
	if d.PingResult == nil {
		return nil
	}

	reading := &probeReading{Result: metadata.ProbeResult{
		Kind:   kindPing,
		Status: statusString(d.PingResult.Success),
		RttMs:  d.PingResult.RttMs,
	}}
	if !d.PingResult.Success {
		reading.Result.FailureReason = d.PingResult.FailureReason
	}
	return reading
}

// probePing reports whether this agent can send ICMP echo requests.
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
