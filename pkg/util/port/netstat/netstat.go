// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

// Package netstat returns the local machine's network connection table.
package netstat

import (
	"errors"
	"net/netip"
	"runtime"
)

// ErrNotImplemented is returned by Get on unsupported platforms.
var ErrNotImplemented = errors.New("not implemented for GOOS=" + runtime.GOOS)

// Entry is a single entry in the connection table.
type Entry struct {
	Local, Remote netip.AddrPort
	Pid           int
	State         string
	OSMetadata    OSMetadata
}

// Table contains local machine's TCP connection entries.
//
// Currently only TCP (IPv4 and IPv6) are included.
type Table struct {
	Entries []Entry
}

// Get returns the connection table.
//
// It returns ErrNotImplemented if the table is not available for the
// current operating system.
func Get() (*Table, error) {
	return get()
}
