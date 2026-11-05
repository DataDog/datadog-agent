// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build linux

package network

import (
	"errors"
	"net"

	"github.com/vishvananda/netlink/nl"
	"golang.org/x/sys/unix"
)

var listAddresses = func() ([][]byte, error) {
	req := nl.NewNetlinkRequest(unix.RTM_GETADDR, unix.NLM_F_DUMP)
	req.AddData(nl.NewIfAddrmsg(unix.AF_UNSPEC))
	return req.Execute(unix.NETLINK_ROUTE, unix.RTM_NEWADDR)
}

func loadInterfaces(ifaces []net.Interface) ([]networkInterface, error) {
	messages, err := listAddresses()
	if err != nil {
		return nil, err
	}

	addresses := make(map[int][]net.Addr, len(ifaces))
	for _, message := range messages {
		index, addr, err := parseAddress(message)
		if err != nil {
			return nil, err
		}
		if addr != nil {
			addresses[index] = append(addresses[index], addr)
		}
	}
	result := make([]networkInterface, len(ifaces))
	for i := range ifaces {
		result[i] = &realNetworkInterface{iface: ifaces[i], addrs: addresses[ifaces[i].Index]}
	}
	return result, nil
}

func parseAddress(message []byte) (int, *net.IPNet, error) {
	if len(message) < unix.SizeofIfAddrmsg {
		return 0, nil, errors.New("short netlink address message")
	}
	msg := nl.DeserializeIfAddrmsg(message)
	index := int(msg.Index)
	addressBytes := net.IPv4len
	switch msg.Family {
	case unix.AF_INET:
	case unix.AF_INET6:
		addressBytes = net.IPv6len
	default:
		return index, nil, nil
	}
	attrs, err := nl.ParseRouteAttrAsMap(message[msg.Len():])
	if err != nil {
		return 0, nil, err
	}
	// IFA_LOCAL is the local IP when IFA_ADDRESS contains a peer address.
	ip := attrs[unix.IFA_LOCAL].Value
	if ip == nil {
		ip = attrs[unix.IFA_ADDRESS].Value
	}
	if len(ip) == 0 {
		return index, nil, nil
	}
	if len(ip) != addressBytes || int(msg.Prefixlen) > 8*addressBytes {
		return 0, nil, errors.New("invalid netlink interface address")
	}
	// Interface.Addrs uses the original prefix with the local IP.
	return index, &net.IPNet{IP: ip, Mask: net.CIDRMask(int(msg.Prefixlen), 8*addressBytes)}, nil
}
