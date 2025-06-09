// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package packets

import (
	"net/netip"
)

// Sink is an interface which sends IP packets
type Sink interface {
	// WriteTo writes the given packet (buffer starts at the IP layer) to addrPort.
	// (the port is required for compatibility with Windows)
	WriteTo(buf []byte, addrPort netip.AddrPort) error
	// Close closes the socket
	Close() error
}
