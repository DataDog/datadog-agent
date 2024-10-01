// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Contains BSD-2-Clause code (c) 2015-present Andrea Barberio

package net

import "net"

// GetLocalAddr returns the local address to reach the IP address with the given network type.
func GetLocalAddr(network string, ip net.IP) (net.Addr, error) {
	// ugly porkaround until I find how to get the local address in a better
	// way. A port different from 0 is required on darwin, so using udp/53.
	conn, err := net.Dial(network, net.JoinHostPort(ip.String(), "53"))
	if err != nil {
		return nil, err
	}
	localAddr := conn.LocalAddr()
	_ = conn.Close()
	return localAddr, nil
}
