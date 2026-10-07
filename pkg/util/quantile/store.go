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
	// count is the sum of the n of the bins, in units of 1<<shift() observations.
	count int
	// scaled stays nil until the store is given more observations than its bins
	// can count, see fit.
	scaled *scaling
}

// scaling is the state of a store whose bins count units of 1<<shift
// observations.
type scaling struct {
	shift uint
	// pending holds, sorted by key, the observations of each key that fall short
	// of a unit. Keeping them rather than rounding every insert makes the units
	// of a key its observations divided by 1<<shift, however they were batched.
	pending []remainder
}

// A remainder holds observations of a key that fall short of a unit.
type remainder struct {
	k Key
	n uint64
}

func (sc *scaling) clone() *scaling {
	if sc == nil {
		return nil
	}
	return &scaling{shift: sc.shift, pending: slices.Clone(sc.pending)}
}

func (sc *scaling) equals(o *scaling) bool {
	if sc == nil || o == nil {
		return sc == o
	}
	return sc.shift == o.shift && slices.Equal(sc.pending, o.pending)
}

// shift returns the scale of the bins: each unit of their n stands for
// 1<<shift observations.
func (s *sparseStore) shift() uint {
	if s.scaled == nil {
		return 0
	}
	return s.scaled.shift
}

// pending returns the observations short of a unit, see scaling.
func (s *sparseStore) pending() []remainder {
	if s.scaled == nil {
		return nil
	}
	return s.scaled.pending
}

// Cols returns an array of k and n.
func (s *sparseStore) Cols() (k []int32, n []uint32) {
	if len(s.bins) == 0 {
		return
	}

	k = make([]int32, len(s.bins))
	n = make([]uint32, len(s.bins))

	// The observations short of a unit are left out. Past maxColsShift the bins
	// keep their proportions, but no longer add up to the number of observations.
	shift := min(s.shift(), maxColsShift)

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
		binSize       = int(unsafe.Sizeof(bin{}))
		storeSize     = int(unsafe.Sizeof(sparseStore{}))
		scalingSize   = int(unsafe.Sizeof(scaling{}))
		remainderSize = int(unsafe.Sizeof(remainder{}))
	)
	// cap is used instead of len because an improved algorithm would take advantage
	// of the unused space after a slice is resized.
	used = storeSize + (len(s.bins) * binSize)
	allocated = storeSize + (cap(s.bins) * binSize)
	if s.scaled != nil {
		used += scalingSize + len(s.scaled.pending)*remainderSize
		allocated += scalingSize + cap(s.scaled.pending)*remainderSize
	}
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

// fit rescales s to the smallest shift at which its bins can count the
// observations of kcs on top of their own.
func (s *sparseStore) fit(c *Config, kcs []KeyCount) {
	capacity := unitCapacity(c)

	shift := s.shift()
	for shift < maxShift && !s.fits(capacity, shift, kcs) {
		shift++
	}

	s.rescale(shift)
}

// fits reports whether, rescaled to shift, the bins of s can count the
// observations of kcs on top of their own. Past a shift of 0, it allows for the
// units the remainders can make up: one per remainder involved, and one more.
func (s *sparseStore) fits(capacity uint64, shift uint, kcs []KeyCount) bool {
	units := uint64(s.count) >> (shift - s.shift())
	if shift > 0 {
		units += uint64(len(s.pending()) + len(kcs) + 1)
	}
	if units > capacity {
		return false
	}

	for _, kc := range kcs {
		// Checking against the room left cannot overflow, unlike a sum.
		u := uint64(kc.n) >> shift
		if u > capacity-units {
			return false
		}
		units += u
	}

	return true
}

// mergeFits is fits, for the observations of o.
func (s *sparseStore) mergeFits(capacity uint64, shift uint, o *sparseStore) bool {
	units := uint64(s.count)>>(shift-s.shift()) + uint64(o.count)>>(shift-o.shift())
	if shift > 0 {
		units += uint64(len(s.pending()) + len(o.pending()) + 1)
	}
	return units <= capacity
}

// rescale converts the bins of s to units of 1<<shift observations. What a key
// no longer has a whole unit for goes to its remainder.
func (s *sparseStore) rescale(shift uint) {
	from := s.shift()
	if shift <= from {
		return
	}

	var (
		by      = shift - from
		pending = s.pending()
		rems    = make([]remainder, 0, len(pending)+1)
		count   int
		p       int
		// A key never needs more bins than it had: rescale them in place.
		bins = s.bins[:0]
	)

	for i := 0; i < len(s.bins); {
		k := s.bins[i].k
		var n uint64
		for ; i < len(s.bins) && s.bins[i].k == k; i++ {
			n += uint64(s.bins[i].n)
		}

		for ; p < len(pending) && pending[p].k < k; p++ {
			rems = append(rems, pending[p])
		}
		rem := (n & (1<<by - 1)) << from
		if p < len(pending) && pending[p].k == k {
			rem += pending[p].n
			p++
		}

		if units := int(n >> by); units > 0 {
			bins = appendSafe(bins, k, units)
			count += units
		}
		if rem > 0 {
			rems = append(rems, remainder{k: k, n: rem})
		}
	}
	rems = append(rems, pending[p:]...)

	s.bins, s.count = bins, count
	s.scaled = &scaling{shift: shift, pending: rems}
}

// carry adds obs, sorted by key, to the remainders of s, and returns the whole
// units they make up, as counts sorted by key. It keeps remainders for at most
// limit keys, folding the lowest ones into the next as trimLeft does for bins.
func (s *sparseStore) carry(obs []remainder, limit int) []KeyCount {
	type entry struct {
		k          Key
		units, rem uint64
	}

	var (
		shift   = s.scaled.shift
		unit    = uint64(1) << shift
		pending = s.scaled.pending
		acc     = make([]entry, 0, len(pending)+len(obs))
		p       int
	)

	add := func(e *entry, n uint64) {
		e.units += n >> shift
		e.rem += n & (unit - 1)
		if e.rem >= unit {
			e.units++
			e.rem -= unit
		}
	}

	for _, o := range obs {
		for ; p < len(pending) && pending[p].k < o.k; p++ {
			acc = append(acc, entry{k: pending[p].k, rem: pending[p].n})
		}
		if len(acc) == 0 || acc[len(acc)-1].k != o.k {
			acc = append(acc, entry{k: o.k})
			if p < len(pending) && pending[p].k == o.k {
				acc[len(acc)-1].rem = pending[p].n
				p++
			}
		}
		add(&acc[len(acc)-1], o.n)
	}
	for ; p < len(pending); p++ {
		acc = append(acc, entry{k: pending[p].k, rem: pending[p].n})
	}

	held := 0
	for _, e := range acc {
		if e.rem > 0 {
			held++
		}
	}
	// With more than limit >= 1 remainders left from i on, another one follows it.
	for i := 0; held > max(limit, 1); i++ {
		if acc[i].rem == 0 {
			continue
		}
		j := i + 1
		for acc[j].rem == 0 {
			j++
		}

		add(&acc[j], acc[i].rem)
		acc[i].rem = 0
		held--
		if acc[j].rem == 0 {
			held--
		}
	}

	var units []KeyCount
	rems := pending[:0]
	for _, e := range acc {
		if e.units > 0 {
			units = append(units, KeyCount{k: e.k, n: uint(e.units)})
		}
		if e.rem > 0 {
			rems = append(rems, remainder{k: e.k, n: e.rem})
		}
	}
	s.scaled.pending = rems

	return units
}

func (s *sparseStore) merge(c *Config, o *sparseStore) {
	// Bring both stores to the smallest shift at which the bins can count the
	// observations of both.
	capacity := unitCapacity(c)
	shift := max(s.shift(), o.shift())
	for shift < maxShift && !s.mergeFits(capacity, shift, o) {
		shift++
	}
	s.rescale(shift)

	// o must not be mutated: rescale a copy of it.
	if shift > o.shift() {
		rescaled := &sparseStore{bins: append(getBinList(), o.bins...), count: o.count, scaled: o.scaled.clone()}
		rescaled.rescale(shift)
		defer putBinList(rescaled.bins)
		o = rescaled
	}

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

	if s.scaled != nil {
		if units := s.carry(o.pending(), c.binLimit); len(units) > 0 {
			s.insertUnits(c, units)
		}
	}
}

func (s *sparseStore) insertCounts(c *Config, kcs []KeyCount) {

	// TODO|PERF: A custom uint16 sort should easily beat sort.Sort.
	// TODO|PERF: Would it be cheaper to sort float64s and then convert to keys?
	sort.Slice(kcs, func(i, j int) bool {
		return kcs[i].k < kcs[j].k
	})

	// appendSafe adds a bin per maxBinWidth of count: scale the counts down when
	// they would take more bins than trimLeft can bound, whatever their size.
	s.fit(c, kcs)
	if s.scaled != nil {
		obs := make([]remainder, len(kcs))
		for i, kc := range kcs {
			obs[i] = remainder{k: kc.k, n: uint64(kc.n)}
		}
		kcs = s.carry(obs, c.binLimit)
	}

	s.insertUnits(c, kcs)
}

// insertUnits merges kcs, sorted by key and counting units, into the bins of s.
func (s *sparseStore) insertUnits(c *Config, kcs []KeyCount) {
	// TODO|PERF: Add a non-allocating fast path. When every key is already contained
	// in the sketch (and no overflow happens) we can just directly update.
	tmp := getBinList()

	var (
		sIdx, keyIdx int
	)

	for sIdx < len(s.bins) && keyIdx < len(kcs) {
		b := s.bins[sIdx]
		vk := kcs[keyIdx].k
		kn := int(kcs[keyIdx].n)

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
			tmp = appendSafe(tmp, b.k, int(b.n)+kn)
			s.count += kn
			sIdx++
			keyIdx++
		}
	}

	tmp = append(tmp, s.bins[sIdx:]...)

	for keyIdx < len(kcs) {
		kn := int(kcs[keyIdx].n)
		tmp = appendSafe(tmp, kcs[keyIdx].k, kn)
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
	if s.scaled != nil || uint64(s.count)+uint64(len(keys)) > unitCapacity(c) {
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
