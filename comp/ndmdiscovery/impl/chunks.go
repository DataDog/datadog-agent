// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package ndmdiscoveryimpl implements the ndmdiscovery component.
package ndmdiscoveryimpl

import (
	"fmt"
	"net/netip"
)

// chunkSize is the number of addresses dispatched to the connectivity engine
// in one request. A /24 is small enough to bound memory and give useful
// progress granularity, and large enough to amortise the request overhead.
const chunkSize = 256

// probeChunk is one unit of work: a contiguous slice of a range's addresses.
type probeChunk struct {
	Index   int
	Targets []string
}

// chunkPlan enumerates the addresses of a CIDR range one chunk at a time.
// It never materialises the whole range, so a /16 costs a 256-entry slice.
type chunkPlan struct {
	prefix  netip.Prefix
	total   int
	ignored map[netip.Addr]struct{}
}

// newChunkPlan builds a plan for cidr, excluding ignored addresses from the
// probed targets. A range holding more than maxAddresses addresses is
// rejected: sweeping it would take longer than any useful cycle.
func newChunkPlan(cidr string, ignored []string, maxAddresses int) (*chunkPlan, error) {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}
	if !prefix.Addr().Is4() {
		return nil, fmt.Errorf("invalid CIDR %q: only IPv4 ranges are supported", cidr)
	}
	prefix = prefix.Masked()

	total := 1 << (32 - prefix.Bits())
	if total > maxAddresses {
		return nil, fmt.Errorf("range %q holds %d addresses, which exceeds the maximum of %d", cidr, total, maxAddresses)
	}

	p := &chunkPlan{
		prefix:  prefix,
		total:   total,
		ignored: make(map[netip.Addr]struct{}, len(ignored)),
	}
	for _, raw := range ignored {
		addr, err := netip.ParseAddr(raw)
		if err != nil {
			// A malformed ignore entry cannot match any enumerated address,
			// so it is dropped rather than failing the whole range.
			continue
		}
		if !p.prefix.Contains(addr) {
			continue
		}
		p.ignored[addr] = struct{}{}
	}
	return p, nil
}

// chunkCount is the number of chunks needed to cover the range.
func (p *chunkPlan) chunkCount() int {
	return (p.total + chunkSize - 1) / chunkSize
}

// totalAddresses is the size of the range, including ignored addresses.
func (p *chunkPlan) totalAddresses() int {
	return p.total
}

// ignoredCount is the number of ignored addresses that fall inside the range.
func (p *chunkPlan) ignoredCount() int {
	return len(p.ignored)
}

// chunk materialises the targets of one chunk. Ignored addresses are left out,
// so a chunk can hold fewer than chunkSize targets. An out-of-bounds index
// yields an empty chunk.
func (p *chunkPlan) chunk(index int) probeChunk {
	c := probeChunk{Index: index}
	if index < 0 || index >= p.chunkCount() {
		return c
	}

	start := index * chunkSize
	end := start + chunkSize
	if end > p.total {
		end = p.total
	}

	addr := p.prefix.Addr()
	for i := 0; i < start; i++ {
		addr = addr.Next()
	}

	c.Targets = make([]string, 0, end-start)
	for i := start; i < end; i++ {
		if _, skip := p.ignored[addr]; !skip {
			c.Targets = append(c.Targets, addr.String())
		}
		addr = addr.Next()
	}
	return c
}
