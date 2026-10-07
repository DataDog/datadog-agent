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

const (
	// maxShift keeps 1<<shift within a uint64.
	maxShift = 63
	// maxColsShift is the largest shift at which a full bin, scaled back to
	// observations, still fits the uint32 counts of Cols.
	maxColsShift = 16
)

var _ memSized = (*sparseStore)(nil)

type sparseStore struct {
	bins binList
	// count is the sum of the n of the bins, in units of 1<<shift observations.
	count int
	// shift is the scale of the bins: each unit of their n stands for 1<<shift
	// observations. It stays 0 until the store is given more observations than its
	// bins can count, see fit.
	shift uint
}

// Cols returns an array of k and n.
func (s *sparseStore) Cols() (k []int32, n []uint32) {
	if len(s.bins) == 0 {
		return
	}

	k = make([]int32, len(s.bins))
	n = make([]uint32, len(s.bins))

	// Past maxColsShift the bins keep their proportions, but no longer add up to
	// the number of observations.
	shift := min(s.shift, maxColsShift)

	// TODO: do this better.
	for i, b := range s.bins {
		k[i] = int32(b.k)
		n[i] = uint32(b.n) << shift
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
func trimLeft(a []bin, maxBucketCap int) []bin {
	// XXX:
	// 1. Work through overflow cause
	// 2. CompressMode enum

	// TODO: Research alternate compression methods
	//
	// (1) Remove closest buckets
	// (2) re-gamma (kinda like hdr histogram)
	if maxBucketCap == 0 || len(a) <= maxBucketCap {
		return a
	}

	var (
		nRemove = len(a) - maxBucketCap

		missing  int
		overflow = getOverflowList()
	)

	// TODO|PROD: Benchmark a better overflow scheme.
	// In theory, if we always have the smaller overflow in the lower bucket, we
	// can guarantee that only 1 extra bin is needed for overflow.
	// For example:

	// fmt = (<k>:<n>[ <k>:<num overflow>])
	//                               NEW        CURRENT
	// 1) (0:1) + (0:max*2)       = (0:1 0:2)  (0:1 0:max 0:max)
	// 2) (0:1 0:max) + (0:max-1) = (0:0 0:2)  (0:max 0:max)
	for i := 0; i < nRemove; i++ {
		missing += int(a[i].n)

		if missing > maxBinWidth {
			overflow = append(overflow, bin{
				k: a[i].k,
				n: maxBinWidth,
			})

			missing -= maxBinWidth
		}
	}

	missing = a[nRemove].incrSafe(missing)
	if missing > 0 {
		overflow = appendSafe(overflow, a[nRemove].k, missing)
	}

	overflowLen := len(overflow)

	copy(a, overflow)
	copy(a[overflowLen:], a[nRemove:])
	putOverflowList(overflow)

	return a[:maxBucketCap+overflowLen]
}

// unitCapacity returns how many units the bins of a store may count. Up to
// c.MaxCount(), trimLeft keeps a store within 2*c.binLimit+1 bins: binLimit bins,
// plus at most binLimit+1 overflow ones it cannot fold. Past it, appendSafe would
// add bins with the count rather than with the keys. A binLimit of 0 disables
// trimming, and with it the bound.
func unitCapacity(c *Config) uint64 {
	if c.binLimit == 0 {
		return math.MaxUint64
	}
	return uint64(c.MaxCount())
}

// A scaler converts observations into units of 1<<shift observations. It rounds
// the running total of the counts it converts rather than each of them, so that
// rounding errors do not pile up along the keys: every rank stays within half a
// unit of its exact value.
type scaler struct {
	shift uint
	// rem holds the observations carried over to the next count, plus half a
	// unit to round to the nearest one.
	rem uint64
}

func newScaler(shift uint) scaler {
	if shift == 0 {
		return scaler{}
	}
	return scaler{shift: shift, rem: 1 << (shift - 1)}
}

// units returns the number of units the next n observations add up to.
func (sc *scaler) units(n uint64) uint64 {
	unit := uint64(1) << sc.shift
	q := n >> sc.shift
	sc.rem += n & (unit - 1)
	if sc.rem >= unit {
		q++
		sc.rem -= unit
	}
	return q
}

// scaledCount returns what n units become once rescaled by 1<<by.
func scaledCount(n int, by uint) uint64 {
	sc := newScaler(by)
	return sc.units(uint64(n))
}

// rescaleBins appends bins, sorted by key, to dst with their n divided by 1<<by,
// and returns them with their count. It merges the bins of each key and drops
// the ones left empty: with by > 0, a key never needs more bins than it had, so
// dst can share the backing array of bins.
func rescaleBins(dst, bins []bin, by uint) ([]bin, int) {
	var (
		sc    = newScaler(by)
		count int
	)

	for i := 0; i < len(bins); {
		k := bins[i].k
		n := 0
		for ; i < len(bins) && bins[i].k == k; i++ {
			n += int(sc.units(uint64(bins[i].n)))
		}

		if n > 0 {
			dst = appendSafe(dst, k, n)
			count += n
		}
	}

	return dst, count
}

// rescale converts the bins of s to units of 1<<shift observations.
func (s *sparseStore) rescale(shift uint) {
	if shift <= s.shift {
		return
	}

	s.bins, s.count = rescaleBins(s.bins[:0], s.bins, shift-s.shift)
	s.shift = shift
}

// fit makes room in s for the counts of kcs, sorted by key. It rescales s to the
// smallest shift at which its bins can count those observations on top of its
// own, and returns the scaler converting kcs to it.
func (s *sparseStore) fit(c *Config, kcs []KeyCount) scaler {
	capacity := unitCapacity(c)

	shift := s.shift
	for shift < maxShift && !s.fits(capacity, shift, kcs) {
		shift++
	}

	s.rescale(shift)
	return newScaler(shift)
}

// fits reports whether, rescaled to shift, the bins of s can count the
// observations of kcs on top of their own.
func (s *sparseStore) fits(capacity uint64, shift uint, kcs []KeyCount) bool {
	units := scaledCount(s.count, shift-s.shift)
	if units > capacity {
		return false
	}

	sc := newScaler(shift)
	for _, kc := range kcs {
		// Checking against the room left cannot overflow, unlike a sum.
		u := sc.units(uint64(kc.n))
		if u > capacity-units {
			return false
		}
		units += u
	}

	return true
}

func (s *sparseStore) merge(c *Config, o *sparseStore) {
	// Bring both stores to the smallest shift at which the bins can count the
	// observations of both.
	capacity := unitCapacity(c)
	shift := max(s.shift, o.shift)
	for shift < maxShift &&
		scaledCount(s.count, shift-s.shift)+scaledCount(o.count, shift-o.shift) > capacity {
		shift++
	}
	s.rescale(shift)

	// o must not be mutated: rescale a copy of its bins.
	obins, ocount := []bin(o.bins), o.count
	if shift > o.shift {
		obins, ocount = rescaleBins(getBinList(), o.bins, shift-o.shift)
		defer putBinList(obins)
	}

	// TODO|PERF: Compare blocky merge with other methods.
	// TODO|PERF: We have essentially unlimited tmp space, can we merge into a
	// dense store and then copy back to the sparse version?
	s.count += ocount
	tmp := getBinList()[:0]

	sIdx := 0
	for _, ob := range obins {

		for sIdx < s.bins.Len() && s.bins[sIdx].k < ob.k {
			tmp = append(tmp, s.bins[sIdx])
			sIdx++
		}

		// done with s
		switch {
		case sIdx >= s.bins.Len(), s.bins[sIdx].k > ob.k:
			tmp = append(tmp, ob)
		case s.bins[sIdx].k == ob.k:
			n := int(ob.n) + int(s.bins[sIdx].n)
			tmp = appendSafe(tmp, ob.k, n)
			sIdx++
		}
	}
	tmp = append(tmp, s.bins[sIdx:]...)
	tmp = trimLeft(tmp, c.binLimit)
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

	// appendSafe adds a bin per maxBinWidth of count: scale the counts down when
	// they would take more bins than trimLeft can bound, whatever their size.
	// Rounding can leave a count with no unit, and its key with no bin.
	sc := s.fit(c, kcs)

	// TODO|PERF: Add a non-allocating fast path. When every key is already contained
	// in the sketch (and no overflow happens) we can just directly update.
	tmp := getBinList()

	var (
		sIdx, keyIdx int
	)

	for sIdx < len(s.bins) && keyIdx < len(kcs) {
		b := s.bins[sIdx]
		vk := kcs[keyIdx].k

		switch {
		case b.k < vk:
			tmp = append(tmp, b)
			sIdx++
		case b.k > vk:
			// When vk[i] == vk[i+1] we need to make sure they go in the same bucket.
			kn := int(sc.units(uint64(kcs[keyIdx].n)))
			if kn > 0 {
				tmp = appendSafe(tmp, vk, kn)
			}
			s.count += kn
			keyIdx++
		default:
			kn := int(sc.units(uint64(kcs[keyIdx].n)))
			tmp = appendSafe(tmp, b.k, int(b.n)+kn)
			s.count += kn
			sIdx++
			keyIdx++
		}
	}

	tmp = append(tmp, s.bins[sIdx:]...)

	for keyIdx < len(kcs) {
		kn := int(sc.units(uint64(kcs[keyIdx].n)))
		if kn > 0 {
			tmp = appendSafe(tmp, kcs[keyIdx].k, kn)
		}
		s.count += kn
		keyIdx++
	}

	tmp = trimLeft(tmp, c.binLimit)

	// TODO|PERF: reallocate if cap(s.bins) >> len(s.bins)
	s.bins = s.bins.ensureLen(len(tmp))
	copy(s.bins, tmp)
	putBinList(tmp)
}

func (s *sparseStore) insert(c *Config, keys []Key) {
	if s.shift > 0 || uint64(s.count)+uint64(len(keys)) > unitCapacity(c) {
		// The keys need scaling, which insertCounts takes care of.
		s.insertKeysAsCounts(c, keys)
		return
	}

	s.count += len(keys)

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
			tmp = appendSafe(tmp, vk, kn)
			keyIdx += kn
		default:
			kn := bufCountLeadingEqual(keys, keyIdx)
			tmp = appendSafe(tmp, b.k, int(b.n)+kn)
			sIdx++
			keyIdx += kn
		}
	}

	tmp = append(tmp, s.bins[sIdx:]...)

	for keyIdx < len(keys) {
		kn := bufCountLeadingEqual(keys, keyIdx)
		tmp = appendSafe(tmp, keys[keyIdx], kn)
		keyIdx += kn
	}

	tmp = trimLeft(tmp, c.binLimit)

	// TODO|PERF: reallocate if cap(s.bins) >> len(s.bins)
	s.bins = s.bins.ensureLen(len(tmp))
	copy(s.bins, tmp)
	putBinList(tmp)
}

// insertKeysAsCounts inserts keys through insertCounts, one KeyCount per key.
func (s *sparseStore) insertKeysAsCounts(c *Config, keys []Key) {
	slices.Sort(keys)

	kcs := getKeyCountList()
	for i := 0; i < len(keys); {
		n := bufCountLeadingEqual(keys, i)
		kcs = append(kcs, KeyCount{k: keys[i], n: uint(n)})
		i += n
	}

	s.insertCounts(c, kcs)
	putKeyCountList(kcs)
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
