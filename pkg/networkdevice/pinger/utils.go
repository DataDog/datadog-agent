package pinger

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"

	probing "github.com/prometheus-community/pro-bing"
)

// RunPing creates a pinger for the requested host and sends the requested number of packets to it
func RunPing(cfg *Config, host string, useRawSocket bool) (*probing.Statistics, error) {
	log.Infof("Running ping for host: %s, useRawSocket: %t\n", host, useRawSocket)
	pinger, err := probing.NewPinger(host)
	if err != nil {
		return nil, err
	}
	pinger.SetPrivileged(useRawSocket)
	pinger.Timeout = cfg.Timeout
	pinger.Interval = cfg.Interval
	pinger.Count = cfg.Count
	err = pinger.Run() // Blocks until finished.
	if err != nil {
		return nil, err
	}
	stats := pinger.Statistics() // get send/receive/duplicate/rtt stats

	return stats, nil
}
