// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package pingprobe probes one address with ICMP echo requests.
package pingprobe

import (
	"fmt"
	"runtime"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/pinger"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/failure"
)

// detectTarget is always reachable, so a failed ping means this process cannot
// send ICMP at all.
const detectTarget = "127.0.0.1"

// Options is one ping probe's configuration.
type Options struct {
	Count        int
	Interval     time.Duration
	Timeout      time.Duration
	UseRawSocket bool
}

// Reading is what the ping probe learned about one address.
type Reading struct {
	Success       bool
	RTT           time.Duration
	FailureReason string
	Error         string
}

// Capability is whether this process can send ICMP echo requests, and how.
type Capability struct {
	Available    bool
	UseRawSocket bool
	Reason       string
}

// Fingerprint is a stable digest of these options.
func (o Options) Fingerprint() string {
	return fmt.Sprintf("ping:count=%d interval=%s timeout=%s raw=%t", o.Count, o.Interval, o.Timeout, o.UseRawSocket)
}

// Run pings one address. A failure is a Reading, not an error.
func Run(target string, opts Options) *Reading {
	p, err := pinger.New(pinger.Config{
		UseRawSocket: opts.UseRawSocket,
		Count:        opts.Count,
		Interval:     opts.Interval,
		Timeout:      opts.Timeout,
	})
	if err != nil {
		return &Reading{
			FailureReason: failure.Unknown,
			Error:         fmt.Sprintf("Failed to create a pinger for host '%s': %s", target, err.Error()),
		}
	}

	res, err := p.Ping(target)
	if err != nil {
		return &Reading{
			FailureReason: failure.Unreachable,
			Error:         fmt.Sprintf("Failed to reach host '%s': %s", target, err.Error()),
		}
	}
	if res == nil || !res.CanConnect {
		return &Reading{
			FailureReason: failure.Unreachable,
			Error:         fmt.Sprintf("Failed to connect to host '%s'", target),
		}
	}

	return &Reading{Success: true, RTT: res.AvgRtt, FailureReason: failure.None}
}

// Detect reports whether this process can ping, and the socket type it must use.
func Detect() Capability {
	var useRawSocket bool
	switch runtime.GOOS {
	case "windows":
		useRawSocket = true
	case "darwin", "linux":
		useRawSocket = false
	default:
		return Capability{Reason: "ping is not supported on " + runtime.GOOS}
	}

	r := Run(detectTarget, Options{
		Count:        1,
		Interval:     time.Second,
		Timeout:      time.Second,
		UseRawSocket: useRawSocket,
	})
	if !r.Success {
		return Capability{Reason: r.Error}
	}
	return Capability{Available: true, UseRawSocket: useRawSocket}
}
