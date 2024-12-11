// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/usm/procnet"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// initializeTupleMaps pre-populates some of our eBPF maps with connection data
// sourced from procfs. This aims to improve the correctness of USM monitoring
// data for connections that are older than the system-probe process.
func initializeTupleMaps(m *ddebpf.Manager) {
	tupleByPidFD, _, err := m.GetMap(tupleByPidFDMap)
	if err != nil {
		return
	}

	pidFDByTuple, _, err := m.GetMap(pidFDByTupleMap)
	if err != nil {
		return
	}

	connections := procnet.GetTCPConnections()
	for _, c := range connections {
		if !c.Laddr.IsValid() || !c.Raddr.IsValid() {
			// This is a safeguard against calling As4() or As6() on a
			// zero netip.Addr value. Note that a zero netip.Addr value is not a
			// valid IP address and it's not the same thing as "0.0.0.0" or "::"
			// which are both valid.
			continue
		}

		ll, lh := util.ToLowHighIP(c.Laddr)
		rl, rh := util.ToLowHighIP(c.Raddr)

		meta := uint32(netebpf.TCP)
		if c.Laddr.Is6() {
			meta |= uint32(netebpf.IPv6)
		}

		pidfd := netebpf.PIDFD{
			Pid: c.PID,
			Fd:  c.FD,
		}

		tuple := http.ConnTuple{
			Saddr_h:  lh,
			Saddr_l:  ll,
			Daddr_h:  rh,
			Daddr_l:  rl,
			Sport:    c.Lport,
			Dport:    c.Rport,
			Netns:    c.NetNS,
			Pid:      c.PID,
			Metadata: meta,
		}

		err := tupleByPidFD.Put(pidfd, tuple)
		if err != nil {
			continue
		}

		err = pidFDByTuple.Put(tuple, pidfd)
		if err != nil {
			tupleByPidFD.Delete(pidfd) //nolint:errcheck
		}
	}
}
