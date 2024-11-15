// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package testutils holds test utility functions
package testutils

import (
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
)

const (
	// CSMDummyInterface is the Dummy interface name used by the IMDS tests
	CSMDummyInterface = "dummy_csm"
)

// CreateDummyInterface creates a dummy interface and attaches it to the provided IP
func CreateDummyInterface(name string, cidr string) (*netlink.Dummy, error) {
	dummy := &netlink.Dummy{
		LinkAttrs: netlink.LinkAttrs{
			Name: name,
		},
	}

	// delete existing dummy interface
	_ = netlink.LinkDel(dummy)

	// Add the dummy interface
	if err := netlink.LinkAdd(dummy); err != nil {
		return nil, fmt.Errorf("failed to create dummy interface %s: %v", name, err)
	}

	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CIDR %s: %v", cidr, err)
	}

	// attach the IMDS IP to the dummy interface
	addr := &netlink.Addr{IPNet: ipNet}
	if err := netlink.AddrAdd(dummy, addr); err != nil {
		return nil, fmt.Errorf("failed to attach IMDS IP to %s: %v", name, err)
	}

	// set dummy interface up
	if err := netlink.LinkSetUp(dummy); err != nil {
		return nil, fmt.Errorf("failed to set %s up: %v", name, err)
	}

	return dummy, nil
}

// RemoveDummyInterface removes the provided dummy interface
func RemoveDummyInterface(link *netlink.Dummy) error {
	if err := netlink.LinkDel(link); err != nil {
		return fmt.Errorf("failed to delete %s: %v", link.Name, err)
	}
	return nil
}
