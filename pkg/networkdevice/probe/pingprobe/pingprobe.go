// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package pingprobe probes one address with ICMP echo requests.
package pingprobe

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/pinger"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/failure"
)

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

// DefaultRawSocket is the socket type goos pings with: Windows only has raw ICMP
// sockets and macOS only unprivileged ones, and Linux defaults to unprivileged.
func DefaultRawSocket(goos string) bool {
	return goos == "windows"
}

// ResolveRawSocket applies a socket-type override on Linux, the only platform that offers a choice.
func ResolveRawSocket(goos string, override *bool) bool {
	if goos == "linux" && override != nil {
		return *override
	}
	return DefaultRawSocket(goos)
}
