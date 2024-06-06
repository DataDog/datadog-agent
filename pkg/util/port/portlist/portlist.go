// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

// This file is just the types. The bulk of the code is in poller.go.

// Package portlist contains code that checks what ports are open and
// listening on the current machine.
package portlist

// Port is a listening port on the machine.
type Port struct {
	Proto   string // "tcp" or "udp"
	Port    uint16 // port number
	Process string // optional process name, if found (requires suitable permissions)
	Pid     int    // process ID, if known (requires suitable permissions)
}

// List is a list of Ports.
type List []Port

func (a *Port) equal(b *Port) bool {
	return a.Port == b.Port &&
		a.Proto == b.Proto &&
		a.Process == b.Process
}

func (a List) equal(b List) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].equal(&b[i]) {
			return false
		}
	}
	return true
}
