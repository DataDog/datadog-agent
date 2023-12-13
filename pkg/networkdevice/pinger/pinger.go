package pinger

import (
	"errors"
	"time"
)

var (
	ErrRawSocketUnsupported = errors.New("raw socket cannot be used with this OS")
	ErrUDPSocketUnsupported = errors.New("udp socket cannot be used with this OS")
)

type (
	// Config defines how pings should be run
	// across all hosts
	Config struct {
		// UseRawSocket determines the socket type to use
		// RAW or UDP
		UseRawSocket bool
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
		Ping(host string) (*Result, error)
	}

	// Result encapsulates the results of a single run
	// of ping
	Result struct {
		// CanConnect indicates if we can connect to the host
		// TODO:(ken) should this be true only if 1/4 packets is good? Should it be percentage based?
		// do we start with only a single ping packet?
		CanConnect bool
		// AvgLatency is the average latency
		AvgLatency time.Duration
	}
)
