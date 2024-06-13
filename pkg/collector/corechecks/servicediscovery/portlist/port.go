// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package portlist

import (
	"sort"
)

// Port is a listening port on the machine.
type Port struct {
	Proto   string `json:"proto"`   // "tcp" or "udp"
	Port    uint16 `json:"port"`    // port number
	Process string `json:"process"` // optional process name, if found (requires suitable permissions)
	Pid     int    `json:"pid"`     // process ID, if known (requires suitable permissions)
}

func (a *Port) equal(b *Port) bool {
	return a.Port == b.Port &&
		a.Proto == b.Proto &&
		a.Process == b.Process
}

func (a *Port) lessThan(b *Port) bool {
	if a.Port != b.Port {
		return a.Port < b.Port
	}
	if a.Proto != b.Proto {
		return a.Proto < b.Proto
	}
	return a.Process < b.Process
}

// List is a list of Ports.
type List []Port

// sortAndDedup sorts ps in place (by Port.lessThan) and then returns
// a subset of it with duplicate (Proto, Port) removed.
func sortAndDedup(ps List) List {
	sort.Slice(ps, func(i, j int) bool {
		return (&ps[i]).lessThan(&ps[j])
	})
	out := ps[:0]
	var last Port
	for _, p := range ps {
		if last.Proto == p.Proto && last.Port == p.Port {
			continue
		}
		out = append(out, p)
		last = p
	}
	return out
}
