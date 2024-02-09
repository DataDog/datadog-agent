/* SPDX-License-Identifier: BSD-2-Clause */

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
