// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package portlist

import "net/netip"

// TCPListener is one TCP listening socket, as reported to NewTCPPoller.
type TCPListener struct {
	IP    netip.Addr // v4-mapped IPv6 addresses are unmapped
	Port  uint16
	Inode uint32
	// Loopback is true for 127.0.0.1 from IPv4 or ::1 from IPv6, matching
	// the /proc/net localhost filter.
	Loopback bool
}

// NewTCPPoller returns a Poller that reports only TCP listeners. On Linux it
// takes them from listeners and reads /proc/net/tcp* only when listeners is
// nil or fails. Other platforms return a regular Poller.
func NewTCPPoller(includeLocalhost bool, listeners func() ([]TCPListener, error)) *Poller {
	p := &Poller{IncludeLocalhost: includeLocalhost}
	p.initTCP(listeners)
	return p
}
