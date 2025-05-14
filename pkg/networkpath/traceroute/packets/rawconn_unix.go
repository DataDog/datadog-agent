// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build unix || linux

package packets

import (
	"context"
	"fmt"
	"net"
	"net/netip"

	"golang.org/x/net/ipv4"
)

// MakeRawConn returns an ipv4.RawConn for the provided network/address
func MakeRawConn(ctx context.Context, lc *net.ListenConfig, network string, localAddr netip.Addr) (*ipv4.RawConn, error) {
	if !localAddr.Is4() {
		return nil, fmt.Errorf("MakeRawConn only supports IPv4 (for now)")
	}
	conn, err := lc.ListenPacket(ctx, network, localAddr.String())
	if err != nil {
		return nil, fmt.Errorf("makeRawConn failed to ListenPacket: %w", err)
	}
	rawConn, err := ipv4.NewRawConn(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("makeRawConn failed to make NewRawConn: %w", err)
	}

	return rawConn, nil
}
