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

func loadInterfaces(ifaces []net.Interface) ([]networkInterface, error) {
	addrs, err := listAddresses(nil, netlink.FAMILY_ALL)
	if err != nil {
		return nil, err
	}

	addresses := make(map[int][]net.Addr, len(ifaces))
	for _, addr := range addrs {
		ipnet := addr.IPNet
		if ipnet == nil || ipnet.IP == nil {
			continue
		}
		if addr.Peer != nil {
			// Interface.Addrs uses the original prefix with the local IP.
			ipnet = &net.IPNet{IP: ipnet.IP, Mask: addr.Peer.Mask}
		}
		addresses[addr.LinkIndex] = append(addresses[addr.LinkIndex], ipnet)
	}
	result := make([]networkInterface, len(ifaces))
	for i := range ifaces {
		result[i] = &realNetworkInterface{iface: ifaces[i], addrs: addresses[ifaces[i].Index]}
	}
	return result, nil
}
