package pinger

import (
	"time"

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
	pinger.Timeout = 3 * time.Second
	pinger.Interval = 1 * time.Second
	pinger.Count = 1
	pinger.SetPrivileged(cfg.UseRawSocket)
	if cfg.Timeout != 0 {
		pinger.Timeout = cfg.Timeout
	}
	if cfg.Interval != 0 {
		pinger.Interval = cfg.Interval
	}
	if cfg.Count != 0 {
		pinger.Count = cfg.Count
	}
	err = pinger.Run() // Blocks until finished.
	if err != nil {
		return &Result{}, err
	}
	stats := pinger.Statistics() // get send/receive/duplicate/rtt stats

	return &Result{
		CanConnect: stats.PacketLoss < 0.50,
		AvgLatency: stats.AvgRtt,
	}, nil
}
