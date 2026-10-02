// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build !linux

package network

import "net"

func loadInterfaceAddresses(ifaces []net.Interface) (map[int]interfaceAddresses, error) {
	addresses := make(map[int]interfaceAddresses, len(ifaces))
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		addresses[iface.Index] = interfaceAddresses{addrs: addrs, err: err}
	}
	return addresses, nil
}
