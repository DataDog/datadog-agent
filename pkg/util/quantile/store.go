// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package quantile

import (
	"math"
	"slices"
	"sort"
	"unsafe"
)

var _ memSized = (*sparseStore)(nil)

type sparseStore struct {
	bins  binList
	count uint64
}

// Cols returns an array of k and n.
func (s *sparseStore) Cols() (k []int32, n []uint32) {
	if len(s.bins) == 0 {
		return
	}

	k = make([]int32, len(s.bins))
	n = make([]uint32, len(s.bins))

	// TODO: do this better.
	for i, b := range s.bins {
		k[i] = int32(b.k)
		n[i] = b.n
	}

	return
}

// MemSize returns memory use in bytes:
//
//	used: uses len(bins)
//	allocated: uses cap(bins)
func (s *sparseStore) MemSize() (used, allocated int) {
	const (
		binSize   = int(unsafe.Sizeof(bin{}))
		storeSize = int(unsafe.Sizeof(sparseStore{}))
	)
	// cap is used instead of len because an improved algorithm would take advantage
	// of the unused space after a slice is resized.
	used = storeSize + (len(s.bins) * binSize)
	allocated = storeSize + (cap(s.bins) * binSize)
	return
}

// trimLeft ensures that len(a) <= maxBucketCap. We set maxBucketCap rather high
// by default to avoid trimming as much as possible.
//
// As CollapsingLowestDenseStore in sketches-go does, the observations of the
// lowest bins collapse into the lowest bin kept. That bin can only count
// maxBinWidth of them: trimLeft returns the observations that did not fit.
func trimLeft(a []bin, maxBucketCap int) ([]bin, uint64) {
	if maxBucketCap == 0 || len(a) <= maxBucketCap {
		return a, 0
	}

	nRemove := len(a) - maxBucketCap

	var missing uint64
	for _, b := range a[:nRemove] {
		missing += uint64(b.n)
	}

	lost := a[nRemove].incrSafe(missing)
	copy(a, a[nRemove:])

	return a[:maxBucketCap], lost
}

// A run holds the observations of a key that appendSafe splits into bins.
type run struct {
	k Key
	n uint64
}

// binsFor returns how many bins appendSafe gives n observations.
func binsFor(n uint64) uint64 {
	if n <= maxBinWidth {
		return 1
	}

	b := n / maxBinWidth
	if n%maxBinWidth != 0 {
		b++
	}
	return b
}

// addSat returns a + b, saturating at the largest uint64.
func addSat(a, b uint64) uint64 {
	if c := a + b; c >= a {
		return c
	}
	return math.MaxUint64
}

// appendTrimmed appends to dst the bins appendSafe gives runs, sorted by key, as
// trimLeft trims them to limit. It only lays out the bins trimLeft keeps, so what
// it allocates does not grow with the counts of runs.
func appendTrimmed(dst []bin, runs []run, limit int) []bin {
	// Walk down from the highest run until there are no more bins to keep.
	first, keep := 0, uint64(limit)
	if limit > 0 {
		for first = len(runs); first > 0; first-- {
			b := binsFor(runs[first-1].n)
			if b > keep {
				break
			}
			keep -= b
		}
	}

	if first == 0 {
		for _, r := range runs {
			dst = appendSafe(dst, r.k, r.n)
		}
		return dst
	}

	// runs[first-1] keeps its highest keep bins, which are full ones. The rest of
	// it, and the runs below, collapse into the lowest bin kept.
	var missing uint64
	for _, r := range runs[:first-1] {
		missing = addSat(missing, r.n)
	}
	r := runs[first-1]
	missing = addSat(missing, r.n-keep*maxBinWidth)

	lowest := len(dst)
	for i := uint64(0); i < keep; i++ {
		dst = append(dst, bin{k: r.k, n: maxBinWidth})
	}
	for _, r := range runs[first:] {
		dst = appendSafe(dst, r.k, r.n)
	}
	dst[lowest].incrSafe(missing)

	return dst
}

func (s *sparseStore) merge(c *Config, o *sparseStore) {
	// TODO|PERF: Compare blocky merge with other methods.
	// TODO|PERF: We have essentially unlimited tmp space, can we merge into a
	// dense store and then copy back to the sparse version?
	s.count += o.count
	tmp := getBinList()[:0]

	sIdx := 0
	for _, ob := range o.bins {

		for sIdx < s.bins.Len() && s.bins[sIdx].k < ob.k {
			tmp = append(tmp, s.bins[sIdx])
			sIdx++
		}

		// done with s
		switch {
		case sIdx >= s.bins.Len(), s.bins[sIdx].k > ob.k:
			tmp = append(tmp, ob)
		case s.bins[sIdx].k == ob.k:
			n := uint64(ob.n) + uint64(s.bins[sIdx].n)
			tmp = appendSafe(tmp, ob.k, n)
			sIdx++
		}
	}
	tmp = append(tmp, s.bins[sIdx:]...)
	tmp, lost := trimLeft(tmp, c.binLimit)
	s.count -= lost
	s.bins = s.bins.ensureLen(len(tmp))
	copy(s.bins, tmp)
	putBinList(tmp)
}

func (s *sparseStore) insertCounts(c *Config, kcs []KeyCount) {

	// TODO|PERF: A custom uint16 sort should easily beat sort.Sort.
	// TODO|PERF: Would it be cheaper to sort float64s and then convert to keys?
	sort.Slice(kcs, func(i, j int) bool {
		return kcs[i].k < kcs[j].k
	})

	// appendSafe gives a count past maxBinWidth as many bins as it takes, which
	// trimLeft would mostly drop: lay out only the bins trimLeft keeps.
	if slices.ContainsFunc(kcs, func(kc KeyCount) bool { return uint64(kc.n) > maxBinWidth }) {
		s.insertLargeCounts(c, kcs)
		return
	}

	// TODO|PERF: Add a non-allocating fast path. When every key is already contained
	// in the sketch (and no overflow happens) we can just directly update.
	tmp := getBinList()

	var (
		sIdx, keyIdx int
	)

	for sIdx < len(s.bins) && keyIdx < len(kcs) {
		b := s.bins[sIdx]
		vk := kcs[keyIdx].k
		kn := uint64(kcs[keyIdx].n)

		switch {
		case b.k < vk:
			tmp = append(tmp, b)
			sIdx++
		case b.k > vk:
			// When vk[i] == vk[i+1] we need to make sure they go in the same bucket.
			tmp = appendSafe(tmp, vk, kn)
			s.count += kn
			keyIdx++
		default:
			tmp = appendSafe(tmp, b.k, uint64(b.n)+kn)
			s.count += kn
			sIdx++
			keyIdx++
		}
	}

	tmp = append(tmp, s.bins[sIdx:]...)

	for keyIdx < len(kcs) {
		kn := uint64(kcs[keyIdx].n)
		tmp = appendSafe(tmp, kcs[keyIdx].k, kn)
		s.count += kn
		keyIdx++
	}

	tmp, lost := trimLeft(tmp, c.binLimit)
	s.count -= lost

	// TODO|PERF: reallocate if cap(s.bins) >> len(s.bins)
	s.bins = s.bins.ensureLen(len(tmp))
	copy(s.bins, tmp)
	putBinList(tmp)
}

// insertLargeCounts is insertCounts for counts that can take any number of bins:
// it merges them with the bins as runs, and only lays out the bins trimLeft keeps.
func (s *sparseStore) insertLargeCounts(c *Config, kcs []KeyCount) {
	runs := getRunList()

	var (
		sIdx, keyIdx int
	)

	for sIdx < len(s.bins) && keyIdx < len(kcs) {
		b := s.bins[sIdx]
		vk := kcs[keyIdx].k
		kn := uint64(kcs[keyIdx].n)

		switch {
		case b.k < vk:
			runs = append(runs, run{k: b.k, n: uint64(b.n)})
			sIdx++
		case b.k > vk:
			runs = append(runs, run{k: vk, n: kn})
			keyIdx++
		default:
			runs = append(runs, run{k: b.k, n: addSat(uint64(b.n), kn)})
			sIdx++
			keyIdx++
		}
	}

	for _, b := range s.bins[sIdx:] {
		runs = append(runs, run{k: b.k, n: uint64(b.n)})
	}

	for _, kc := range kcs[keyIdx:] {
		runs = append(runs, run{k: kc.k, n: uint64(kc.n)})
	}

	tmp := appendTrimmed(getBinList(), runs, c.binLimit)
	putRunList(runs)

	s.bins = s.bins.ensureLen(len(tmp))
	copy(s.bins, tmp)
	// Counts that large can add up past what the bins count.
	s.count = s.bins.nSum()
	putBinList(tmp)
}

func (s *sparseStore) insert(c *Config, keys []Key) {
	s.count += uint64(len(keys))

	// TODO|PERF: A custom uint16 sort should easily beat slices.Sort.
	// TODO|PERF: Would it be cheaper to sort float64s and then convert to keys?
	slices.Sort(keys)

	// TODO|PERF: Add a non-allocating fast path. When every key is already contained
	// in the sketch (and no overflow happens) we can just directly update.
	tmp := getBinList()

	var (
		sIdx, keyIdx int
	)

	for sIdx < len(s.bins) && keyIdx < len(keys) {
		b := s.bins[sIdx]
		vk := keys[keyIdx]

		switch {
		case b.k < vk:
			tmp = append(tmp, b)
			sIdx++
		case b.k > vk:
			// When vk[i] == vk[i+1] we need to make sure they go in the same bucket.
			kn := bufCountLeadingEqual(keys, keyIdx)
			tmp = appendSafe(tmp, vk, uint64(kn))
			keyIdx += kn
		default:
			kn := bufCountLeadingEqual(keys, keyIdx)
			tmp = appendSafe(tmp, b.k, uint64(b.n)+uint64(kn))
			sIdx++
			keyIdx += kn
		}
	}

	tmp = append(tmp, s.bins[sIdx:]...)

	for keyIdx < len(keys) {
		kn := bufCountLeadingEqual(keys, keyIdx)
		tmp = appendSafe(tmp, keys[keyIdx], uint64(kn))
		keyIdx += kn
	}

	tmp, lost := trimLeft(tmp, c.binLimit)
	s.count -= lost

	// TODO|PERF: reallocate if cap(s.bins) >> len(s.bins)
	s.bins = s.bins.ensureLen(len(tmp))
	copy(s.bins, tmp)
	putBinList(tmp)
}

// bufCountLeadingEqual returns the number of consecutive keys in a[i:] that equal a[i].
// given:
//
//	i = 0 1 2 3 4 5 6
//	a = 4 5 6 8 8 8 9
//
// bufCountLeadingEqual(a, 3) = 3, a[3] = 8
// bufCountLeadingEqual(a, 4) = 2, a[4] = 8
func bufCountLeadingEqual(a []Key, start int) int {
	if start == len(a)-1 {
		return 1
	}

	i := start
	for i < len(a) && a[i] == a[start] {
		i++
	}

	return i - start
}
