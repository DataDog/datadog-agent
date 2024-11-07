// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils holds utils related files
package utils

import (
	"fmt"
	"net"
	"slices"
)

type byteMaskFilter struct {
	mask byte
	ip   byte
	next []*byteMaskFilter
}

// CIDRSet defines a set of CIDRs
type CIDRSet struct {
	cidrList  []string
	cidrGraph []*byteMaskFilter
}

func appendByteFilter(filter *[]*byteMaskFilter, iter int, ipnet *net.IPNet) {
	if iter > len(ipnet.IP) {
		// we already append all needed filters
		return
	}

	if ipnet.Mask[iter] == 0 {
		// filter already pushed
		return
	}

	last := false
	if ipnet.Mask[iter] != 0xFF ||
		(ipnet.Mask[iter] == 0xFF && iter+1 < len(ipnet.IP) && ipnet.Mask[iter+1] == 0) {
		// last filter
		last = true
	}

	// check filter is not already present:
	for _, f := range *filter {
		if f.mask == ipnet.Mask[iter] && f.ip == ipnet.IP[iter] {
			// found
			appendByteFilter(&f.next, iter+1, ipnet)
			return
		}
	}
	newBMF := &byteMaskFilter{
		mask: ipnet.Mask[iter],
		ip:   ipnet.IP[iter],
		next: []*byteMaskFilter{},
	}
	*filter = append(*filter, newBMF)
	if last {
		return
	}
	appendByteFilter(&newBMF.next, iter+1, ipnet)
}

// AppendCIDR appends a CIDR to the set
func (cs *CIDRSet) AppendCIDR(cidr string) error {
	if slices.Contains(cs.cidrList, cidr) {
		return nil // already present
	}

	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}

	appendByteFilter(&cs.cidrGraph, 0, ipnet)
	cs.cidrList = append(cs.cidrList, cidr)
	return nil
}

func debugFilter(filter *byteMaskFilter, prefix string) {
	fmt.Printf("%s . ip/mask: %d/%d\n", prefix, filter.ip, filter.mask)
	for _, f := range filter.next {
		debugFilter(f, prefix+"  ")
	}
}

// Debug prints on stdout the content of the CIDR set
func (cs *CIDRSet) Debug() {
	fmt.Printf("List of %d CIDR:\n", len(cs.cidrList))
	for _, cidr := range cs.cidrList {
		fmt.Printf("  - %s\n", cidr)
	}
	fmt.Println("Filter graph:")
	for _, f := range cs.cidrGraph {
		debugFilter(f, "  ")
	}
}

func matchByteMask(maskFilter *[]*byteMaskFilter, iter int, ip []byte) bool {
	if len(*maskFilter) == 0 {
		// no more filters
		return true
	}

	for _, f := range *maskFilter {
		if f.ip == (ip[iter] & f.mask) {
			// match
			return matchByteMask(&f.next, iter+1, ip)
		}
	}
	// no match
	return false
}

// MatchIP returns true if the given IP match the CIDR set
func (cs *CIDRSet) MatchIP(ipstring string) bool {
	ipnet := net.ParseIP(ipstring)
	if ipnet == nil {
		return false
	}
	if ipv4 := ipnet.To4(); ipv4 != nil {
		return matchByteMask(&cs.cidrGraph, 0, ipv4)
	} else if ipv6 := ipnet.To16(); ipv6 != nil {
		return matchByteMask(&cs.cidrGraph, 0, ipv6)
	}
	return false
}
