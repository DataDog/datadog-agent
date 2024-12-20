// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package portlist

import (
	"net/netip"
)

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

// GetConnTable returns the connection table.
//
// It returns ErrNotImplemented if the table is not available for the
// current operating system.
func GetConnTable() (*Table, error) {
	return getConnTable()
}
