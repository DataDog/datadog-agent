// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package failure

import (
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf"
)

// Conn represents a failed connection
type Conn ebpf.FailedConn

// Tuple returns a network.ConnectionTuple
func (c Conn) Tuple() network.ConnectionTuple {
	ct := network.ConnectionTuple{
		Source: c.Tup.SourceAddress(),
		Dest:   c.Tup.DestAddress(),
		Pid:    c.Tup.Pid,
		NetNS:  c.Tup.Netns,
		SPort:  c.Tup.Sport,
		DPort:  c.Tup.Dport,
	}

	if c.Tup.Type() == ebpf.TCP {
		ct.Type = network.TCP
	} else {
		ct.Type = network.UDP
	}

	switch c.Tup.Family() {
	case ebpf.IPv4:
		ct.Family = network.AFINET
	case ebpf.IPv6:
		ct.Family = network.AFINET6
	}

	return ct
}
