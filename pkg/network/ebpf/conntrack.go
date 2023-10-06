// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ebpf

import (
	"fmt"
	"net"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// Family returns whether a tuple is IPv4 or IPv6
func (t ConntrackTuple) Family() ConnFamily {
	if t.Metadata&uint32(IPv6) != 0 {
		return IPv6
	}
	return IPv4
}

// Type returns whether a tuple is TCP or UDP
func (t ConntrackTuple) Type() ConnType {
	if t.Metadata&uint32(TCP) != 0 {
		return TCP
	}
	return UDP
}

// SourceAddress returns the source address
func (t ConntrackTuple) SourceAddress() util.Address {
	if t.Family() == IPv6 {
		return util.V6Address(t.Saddr_l, t.Saddr_h)
	}
	return util.V4Address(uint32(t.Saddr_l))
}

// SourceEndpoint returns the source address and source port joined
func (t ConntrackTuple) SourceEndpoint() string {
	return net.JoinHostPort(t.SourceAddress().String(), strconv.Itoa(int(t.Sport)))
}

// DestAddress returns the destination address
func (t ConntrackTuple) DestAddress() util.Address {
	if t.Family() == IPv6 {
		return util.V6Address(t.Daddr_l, t.Daddr_h)
	}
	return util.V4Address(uint32(t.Daddr_l))
}

// DestEndpoint returns the destination address and source port joined
func (t ConntrackTuple) DestEndpoint() string {
	return net.JoinHostPort(t.DestAddress().String(), strconv.Itoa(int(t.Dport)))
}

func (t ConntrackTuple) String() string {
	return fmt.Sprintf(
		"[%s%s] [%s ⇄ %s] (ns: %d)",
		t.Type(),
		t.Family(),
		t.SourceEndpoint(),
		t.DestEndpoint(),
		t.Netns,
	)
}
