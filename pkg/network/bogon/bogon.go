// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// CREDIT https://github.com/lrstanley/go-bogon
// Copyright (c) Liam Stanley <me@liamstanley.io>. All rights reserved. Use
// of this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package bogon provides a simply interface to check if an IP address is
// a "bogon" IP (an internal IP that should not be hitting external services),
// or a custom specified range of CIDR's.
//
// Note that you can use bogon.New() to check your own ranges.
package bogon

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

var defaultCIDRs = DefaultRanges()

// DefaultRanges returns the default list of bogon IP CIDRs.
func DefaultRanges() []*net.IPNet {
	out := []*net.IPNet{
		// IPv4 ranges.
		//MustCIDR("0.0.0.0/8"),
		MustCIDR("10.0.0.0/8"),
		//MustCIDR("100.64.0.0/10"),
		//MustCIDR("127.0.0.0/8"),
		//MustCIDR("169.254.0.0/16"),
		//MustCIDR("172.16.0.0/12"),
		//MustCIDR("192.0.0.0/24"),
		//MustCIDR("192.0.2.0/24"),
		//MustCIDR("192.168.0.0/16"),
		//MustCIDR("198.18.0.0/15"),
		//MustCIDR("198.51.100.0/24"),
		//MustCIDR("203.0.113.0/24"),
		//MustCIDR("224.0.0.0/3"),
		//// IPv6 ranges.
		//// MustCIDR("::/0"),
		//// MustCIDR("::/96"),
		//// MustCIDR("::/128"),
		//// MustCIDR("::1/128"),
		//// MustCIDR("::ffff:0.0.0.0/96"),
		//// MustCIDR("::224.0.0.0/100"),
		//// MustCIDR("::127.0.0.0/104"),
		//// MustCIDR("::0.0.0.0/104"),
		//// MustCIDR("::255.0.0.0/104"),
		//MustCIDR("0000::/8"),
		//MustCIDR("0200::/7"),
		//MustCIDR("3ffe::/16"),
		//MustCIDR("2001:db8::/32"),
		//MustCIDR("2002:e000::/20"),
		//MustCIDR("2002:7f00::/24"),
		//MustCIDR("2002:0000::/24"),
		//MustCIDR("2002:ff00::/24"),
		//MustCIDR("2002:0a00::/24"),
		//MustCIDR("2002:ac10::/28"),
		//MustCIDR("2002:c0a8::/32"),
		//MustCIDR("fc00::/7"),
		//MustCIDR("fe80::/10"),
		//MustCIDR("fec0::/10"),
		//MustCIDR("ff00::/8"),
	}

	return out
}

// MustCIDR converts a string representation of a CIDR notation, to a net.IPNet.
// If it is an invalid CIDR, MustCIDR will throw a panic.
func MustCIDR(cidr string) *net.IPNet {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(fmt.Sprintf("%s is not a valid CIDR", cidr))
	}

	return ipnet
}

// Bogon is a helper utility to use your own IP ranges.
type Bogon struct {
	ipRange []*net.IPNet
}

// Ranges returns the underlying IP CIDR ranges used for checks within Bogon.
func (b *Bogon) Ranges() []*net.IPNet {
	return b.ipRange
}

// String returns a string representation of each CIDR within Bogon.
func (b *Bogon) String() string {
	out := make([]string, len(b.ipRange))

	for i := 0; i < len(b.ipRange); i++ {
		out[i] = b.ipRange[i].String()
	}

	return strings.Join(out, " ")
}

// Is checks if the IP address is within the custom Bogon IP ranges specified
// during the creation of the Bogon instance. representation is non-nil if
// the match is a success, and it contains the string representation of the
// CIDR notation that it matched.
func (b *Bogon) Is(ip string) (isIn bool, representation string) {
	ipAddress := net.ParseIP(ip)
	if ipAddress == nil || ipAddress.IsUnspecified() {
		return false, ""
	}

	for i := 0; i < len(b.ipRange); i++ {
		if b.ipRange[i].Contains(ipAddress) {
			return true, b.ipRange[i].String()
		}
	}

	return false, ""
}

// New returns a new Bogon instance. Use this if you have your own CIDR
// ranges that you would like to check. An error is returned if a nil list
// is supplied or if one of the supplied CIDR's is invalid.
func New(cidrList []string) (*Bogon, error) {
	if cidrList == nil || len(cidrList) < 1 {
		return nil, errors.New("supplied nil cidr ranges")
	}

	list := make([]*net.IPNet, len(cidrList))
	for i, cidr := range cidrList {
		_, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("unable to convert %s to cidr: %s", cidr, err)
		}

		list[i] = ipnet
	}

	return &Bogon{ipRange: list}, nil
}

// Is checks if the IP address is within the default Bogon IP ranges.
// Representation is non-nil if the match is a success, and it contains the
// string representation of the CIDR notation that it matched.
//
// Check the main docs for which IP's are checked within this.
func Is(ip string) (isIn bool, representation string) {
	ipAddress := net.ParseIP(ip)
	if ipAddress == nil || ipAddress.IsUnspecified() {
		return false, ""
	}

	for i := 0; i < len(defaultCIDRs); i++ {
		if defaultCIDRs[i].Contains(ipAddress) {
			return true, defaultCIDRs[i].String()
		}
	}

	return false, ""
}
