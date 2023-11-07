package pinger

import probing "github.com/prometheus-community/pro-bing"

// Pinger is an interface for sending an ICMP ping to a host
type Pinger interface {
	Ping(host string) (*probing.Statistics, error)
}
