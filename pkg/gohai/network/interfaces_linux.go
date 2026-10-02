// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build linux

package network

import (
	"net"

	"github.com/vishvananda/netlink"
)

var listAddresses = netlink.AddrList

func loadInterfaceAddresses(ifaces []net.Interface) (map[int]interfaceAddresses, error) {
	addrs, err := listAddresses(nil, netlink.FAMILY_ALL)
	if err != nil {
		return nil, err
	}

	addresses := make(map[int]interfaceAddresses, len(ifaces))
	for _, addr := range addrs {
		ipnet := addr.IPNet
		if ipnet == nil || ipnet.IP == nil {
			continue
		}
		if addr.Peer != nil {
			// Interface.Addrs uses the original prefix with the local IP.
			ipnet = &net.IPNet{IP: ipnet.IP, Mask: addr.Peer.Mask}
		}
		entry := addresses[addr.LinkIndex]
		entry.addrs = append(entry.addrs, ipnet)
		addresses[addr.LinkIndex] = entry
	}
	return addresses, nil
}
