// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pinger

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"

	probing "github.com/prometheus-community/pro-bing"
)

// RunPing creates a pinger for the requested host and sends the requested number of packets to it
func RunPing(cfg *Config, host string) (*Result, error) {
	log.Infof("Running ping for host: %s, useRawSocket: %t\n", host, cfg.UseRawSocket)
	pinger, err := probing.NewPinger(host)
	if err != nil {
		return &Result{}, err
	}
	// Default configurations
	pinger.Timeout = defaultTimeout
	pinger.Interval = defaultInterval
	pinger.Count = defaultCount
	pinger.SetPrivileged(cfg.UseRawSocket)
	if cfg.Timeout > 0 {
		pinger.Timeout = cfg.Timeout
	}
	if cfg.Interval > 0 {
		pinger.Interval = cfg.Interval
	}
	if cfg.Count > 0 {
		pinger.Count = cfg.Count
	}
	err = pinger.Run() // Blocks until finished.
	if err != nil {
		return &Result{}, err
	}
	stats := pinger.Statistics() // get send/receive/duplicate/rtt stats

	return &Result{
		CanConnect: stats.PacketsRecv > 0,
		PacketLoss: stats.PacketLoss,
		AvgRtt:     stats.AvgRtt,
	}, nil
}
