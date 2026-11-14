// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package quantile

import (
	"math"
	"strings"
)

const (
	maxBinWidth = math.MaxUint32
)

type bin struct {
	k Key
	n uint32
}

// incrSafe performs `b.n += by` safely handling overflows. When an overflow
// occurs, we set b.n to it's max, and return the leftover amount to increment.
func (b *bin) incrSafe(by uint64) uint64 {
	if room := maxBinWidth - uint64(b.n); by > room {
		b.n = maxBinWidth
		return by - room
	}

	b.n += uint32(by)
	return 0
}

// appendSafe appends 1 or more bins with the given key safely handing overflow by
// inserting multiple buckets when needed.
//
//	(1) n <= maxBinWidth :  1 bin
//	(2) n > maxBinWidth  : >1 bin
func appendSafe(bins []bin, k Key, n uint64) []bin {
	if n <= maxBinWidth {
		return append(bins, bin{k: k, n: uint32(n)})
	}

	// on overflow, insert multiple bins with the same key.
	// put full bins at end

	// TODO|PROD: Add validation func that sorts by key and then n (smaller bin first).
	r := uint32(n % maxBinWidth)
	if r != 0 {
		bins = append(bins, bin{k: k, n: r})
	}

	for i := uint64(0); i < n/maxBinWidth; i++ {
		bins = append(bins, bin{k: k, n: maxBinWidth})
	}

	return bins
}

type binList []bin

func (bins binList) nSum() uint64 {
	var s uint64
	for _, b := range bins {
		s += uint64(b.n)
	}
	return s
}

func (bins binList) Cap() int {
	return cap(bins)
}

func (bins binList) Len() int {
	return len(bins)
}

func (bins binList) ensureLen(newLen int) binList {
	for cap(bins) < newLen {
		bins = append(bins[:cap(bins)], bin{})
	}

	return bins[:newLen]
}

func (bins binList) String() string {
	var w strings.Builder
	printBins(&w, bins, defaultBinPerLine)
	return w.String()
}
