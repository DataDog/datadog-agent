package pinger

import (
	"time"

	probing "github.com/prometheus-community/pro-bing"
)

type (
	// Config defines how pings should be run
	// across all hosts
	Config struct {
		// Interval is the amount of time to wait between
		// sending ICMP packets, default is 1 second
		Interval time.Duration
		// Timeout is the total time to wait for all pings
		// to complete
		Timeout time.Duration
		// Count is the number of ICMP packets, pings, to send
		Count int
	}

	// Pinger is an interface for sending an ICMP ping to a host
	Pinger interface {
		Ping(host string) (*probing.Statistics, error)
	}

	// Result encapsulates the results of a single run
	// of ping
	Result struct {
		// CanConnect indicates if we can connect to the host
		// TODO:(ken) should this be true only if 1/4 packets is good? Should it be percentage based?
		// do we start with only a single ping packet?
		CanConnect bool
		// AvgLatency is the average latency
		AvgLatency float64
	}
)
