package pinger

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	probing "github.com/prometheus-community/pro-bing"
)

func RunPing(host string, privileged bool) (*probing.Statistics, error) {
	log.Infof("Running ping for host: %s, privileged: %t\n", host, privileged)
	pinger, err := probing.NewPinger(host)
	if err != nil {
		return nil, err
	}
	pinger.SetPrivileged(privileged)
	pinger.Timeout = 3 * time.Second
	pinger.Count = 3
	err = pinger.Run() // Blocks until finished.
	if err != nil {
		return nil, err
	}
	stats := pinger.Statistics() // get send/receive/duplicate/rtt stats

	return stats, nil
}
