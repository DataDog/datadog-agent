// Prototype metric encoding between clients and the agent
//
// Read a dogstatsd-capture output, aggregate metrics like they would
// be aggregated by a client and write several variants of wire
// encoding to compare size and compressiblity.

package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"iter"
	"maps"
	"math"
	"os"
	"runtime"
	"runtime/pprof"
	"slices"
	stdsort "sort"
	"strconv"
	"strings"
	"time"

	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/impl"
	"github.com/DataDog/datadog-agent/pkg/util/sort"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/quantile"
	"github.com/DataDog/zstd"
	"github.com/richardartoul/molecule"
)

type sketchMode int

const (
	Sketch    sketchMode = 0
	Histogram            = 1
	Timing               = 2
)

type sketch struct {
	sk   *quantile.Agent
	mode sketchMode
}

type bucket struct {
	containerId string
	total       int

	gauges   map[string]map[string]float64
	counts   map[string]map[string]float64
	sketches map[string]map[string]sketch
}

func (b *bucket) length() int {
	if b.total == 0 {
		for _, v := range b.gauges {
			b.total += len(v)
		}
		for _, v := range b.counts {
			b.total += len(v)
		}
		for _, v := range b.sketches {
			b.total += len(v)
		}
	}
	return b.total
}

func (b *bucket) read(payload string) {
}

func (b *bucket) insert(ty, k1, k2 string, numvalues []float64) {
	mode := Sketch
	switch ty {
	case "g":
		if b.gauges == nil {
			b.gauges = make(map[string]map[string]float64)
		}
		l1, ok := b.gauges[k1]
		if !ok {
			l1 = make(map[string]float64)
			b.gauges[k1] = l1
		}
		l1[k2] = numvalues[len(numvalues)-1]
	case "c":
		if b.counts == nil {
			b.counts = make(map[string]map[string]float64)
		}
		l1, ok := b.counts[k1]
		if !ok {
			l1 = make(map[string]float64)
			b.counts[k1] = l1
		}
		for _, v := range numvalues {
			l1[k2] += v
		}
	case "ms":
		mode = Timing
		fallthrough
	case "h":
		mode = Histogram
		fallthrough
	case "d":
		if b.sketches == nil {
			b.sketches = make(map[string]map[string]sketch)
		}
		l1, ok := b.sketches[k1]
		if !ok {
			l1 = make(map[string]sketch)
			b.sketches[k1] = l1
		}
		sk, ok := l1[k2]
		if !ok {
			sk = sketch{mode: mode, sk: &quantile.Agent{}}
			l1[k2] = sk
		}
		for _, v := range numvalues {
			sk.sk.Insert(v, 1.0)
		}
	}
}

func (b *bucket) printStats() {
	if b == nil {
		return
	}

	mapStat("gauges", b.gauges)
	mapStat("counts", b.counts)
	mapStat("sketches", b.sketches)
}

func mapStat[T any](what string, m map[string]map[string]T) {
	total := 0
	if m == nil {
		return
	}
	for _, inner := range m {
		total += len(inner)
	}
	fmt.Printf("-- %s: %d names, %d total\n", what, len(m), total)
}

type interner struct {
	si map[string]int64
	s  []string
	sf map[string]int64

	ti map[string]int64
	t  [][]int64

	g map[string]string
}

func newInterner() interner {
	return interner{
		si: make(map[string]int64),
		sf: make(map[string]int64),
		ti: make(map[string]int64),
		g:  make(map[string]string),
	}
}

func (i *interner) serialize(ps *molecule.ProtoStream) error {
	strs := strings.Join(i.s, "\x00")

	fmt.Printf("-- %d bytes in %d strings\n", len(strs), len(i.s))
	ps.String(1, strs)

	fmt.Printf("-- %d tagsets\n", len(i.t))

	if len(i.g) > 0 {
		fmt.Printf("-- %d global tags\n", len(i.g))
		ss := slices.Collect(maps.Values(i.g))
		slices.Sort(ss)
		ps.String(3, strings.Join(ss, "\x00"))
	}

	for _, t := range i.t {
		ps.Int64Packed(2, t)
	}

	return nil
}

func (i *interner) intern(s string) int64 {
	i.sf[s]++

	if k, ok := i.si[s]; ok {
		return k
	}

	k := int64(len(i.s))
	i.si[s] = k
	i.s = append(i.s, s)
	return k
}

func (i *interner) internTags(s string) []int64 {
	is := []int64{}
	for _, s := range strings.Split(s, ",") {
		is = append(is, i.intern(s))
	}
	slices.Sort(is)
	return is
}

func (i *interner) internTags2(s string) int64 {
	if k, ok := i.ti[s]; ok {
		return k
	}

	is := []int64{}
	for _, s := range strings.Split(s, ",") {
		is = append(is, i.intern(s))
	}

	return i.internTagset(s, is)
}

func (i *interner) internTagset(s string, is []int64) int64 {
	slices.Sort(is)
	deltaEncode(is)

	k := int64(len(i.t))
	i.ti[s] = k
	i.t = append(i.t, is)
	return k
}

var globalTags = func() map[string]struct{} {
	m := map[string]struct{}{}
	for _, k := range []string{
		"app",
		"datacenter",
		"env",
		"site",
		//"service",
		//"client",
		//"client_transport",
		//"client_version",
		"dd.internal.entity_id",
	} {
		m[k] = struct{}{}
	}
	return m
}()

// like intern tags2 but extract global tags first
func (i *interner) internTags3(s string) int64 {
	if k, ok := i.ti[s]; ok {
		return k
	}

	is := []int64{}
	for _, s := range strings.Split(s, ",") {
		pair := strings.SplitN(s, ":", 2)
		if _, ok := globalTags[pair[0]]; ok {
			if prev, ok := i.g[pair[0]]; ok && prev != s {
				panic(fmt.Sprintf("non-unique global tag %q", s))
			}
			i.g[pair[0]] = s
		} else {
			is = append(is, i.intern(s))
		}
	}

	if false {
		if len(is) > 2 {
			prev := is[0]
			for ix := 1; ix < len(is); ix++ {
				if prev == is[ix] {
					fmt.Printf("duplicate tag %d %q in tagset %q\n", prev, i.s[prev], s)
					panic("bugs")
				}
				prev = is[ix]
			}
		}
	}

	return i.internTagset(s, is)
}

func (i *interner) sort() {
	slices.Sort(i.s)
	for j, s := range i.s {
		i.si[s] = int64(j)
	}
}

func (i *interner) sortByFreq() {
	slices.SortFunc(i.s, func(n, m string) int { return int(i.sf[m] - i.sf[n]) })
	for j, s := range i.s {
		i.si[s] = int64(j)
	}
}

func deltaEncode[T int64 | int32](is []T) {
	for i := len(is) - 1; i > 0; i-- {
		is[i] -= is[i-1]
	}
}

const (
	gaugeKind  = 0
	countKind  = 1
	sketchKind = 2 // 3, 4
)

/*
flat, row based

raw = 81039185, t1 = 0.47638473
gz1 = 5959866, t2 = 0.275167146
*/
func (b *bucket) serialize01(w *bytes.Buffer) error {
	type record struct {
		k1, k2 string
		kind   int
		value  float64
		sketch sketch
	}

	recs := []record{}
	for k1, l1 := range b.gauges {
		for k2, val := range l1 {
			recs = append(recs, record{k1: k1, k2: k2, kind: gaugeKind, value: val})
		}
	}
	for k1, l1 := range b.counts {
		for k2, val := range l1 {
			recs = append(recs, record{k1: k1, k2: k2, kind: countKind, value: val})
		}
	}
	for k1, l1 := range b.sketches {
		for k2, val := range l1 {
			recs = append(recs, record{k1: k1, k2: k2, kind: sketchKind, sketch: val})
		}
	}

	slices.SortFunc(recs, func(a, b record) int {
		if a.k1 < b.k1 {
			return -1
		}
		if a.k1 > b.k1 {
			return 1
		}
		if a.k2 < b.k2 {
			return -1
		}
		if a.k2 > b.k2 {
			return 1
		}
		return 0
	})

	ps := molecule.NewProtoStream(w)
	ps.String(2, b.containerId)
	for _, r := range recs {
		ps.Embedded(3, func(ps *molecule.ProtoStream) error {
			ps.String(1, r.k1)
			ps.String(2, r.k2)
			switch r.kind {
			case gaugeKind:
				ps.Double(4, r.value)
			case countKind:
				ps.Double(5, r.value)
			case sketchKind:
				ps.Embedded(6+int(r.sketch.mode), func(ps *molecule.ProtoStream) error {
					sk := r.sketch.sk.Finish()
					b := sk.Basic
					k, n := sk.Cols()
					ps.Int64(2, b.Cnt)
					ps.Double(3, b.Min)
					ps.Double(4, b.Max)
					ps.Double(5, b.Avg)
					ps.Double(6, b.Sum)
					ps.Sint32Packed(7, k)
					ps.Uint32Packed(8, n)
					return nil
				})
			}
			return nil
		})
	}

	return nil
}

/*
flat row format, with strings in a dictionary

-- 267059 bytes in 18284 strings
-- 0 tagsets
-- 267067 header bytes
-- 12593441 metric bytes
raw = 12860508, t1 = 0.832152117
gz1 = 4255025, t2 = 0.131457707
*/
func (b *bucket) serialize02(w *bytes.Buffer) error {
	i := newInterner()

	type record struct {
		name   string
		tags   string
		kind   int
		value  float64
		sketch sketch
	}

	recs := []record{}
	for k1, l1 := range b.gauges {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: gaugeKind, value: val})
		}
	}
	for k1, l1 := range b.counts {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: countKind, value: val})
		}
	}
	for k1, l1 := range b.sketches {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: sketchKind, sketch: val})
		}
	}

	slices.SortFunc(recs, func(a, b record) int {
		if a.name < b.name {
			return -1
		}
		if a.name > b.name {
			return 1
		}
		if a.tags < b.tags {
			return -1
		}
		if a.tags > b.tags {
			return 1
		}
		return 0
	})

	for _, r := range recs {
		i.intern(r.name)
		i.internTags(r.tags)
	}

	ps := molecule.NewProtoStream(w)
	ps.String(2, b.containerId)
	ps.Embedded(4, i.serialize)

	lenHeader := w.Len()
	fmt.Printf("-- %d header bytes\n", lenHeader)

	for _, r := range recs {
		ps.Embedded(3, func(ps *molecule.ProtoStream) error {
			ps.Int64(1, i.intern(r.name))
			ps.Int64Packed(2, i.internTags(r.tags))
			switch r.kind {
			case gaugeKind:
				ps.Double(4, r.value)
			case countKind:
				ps.Double(5, r.value)
			case sketchKind:
				ps.Embedded(6+int(r.sketch.mode), func(ps *molecule.ProtoStream) error {
					sk := r.sketch.sk.Finish()
					b := sk.Basic
					k, n := sk.Cols()
					deltaEncode(k)
					ps.Int64(2, b.Cnt)
					ps.Double(3, b.Min)
					ps.Double(4, b.Max)
					ps.Double(5, b.Avg)
					ps.Double(6, b.Sum)
					ps.Sint32Packed(7, k)
					ps.Uint32Packed(8, n)
					return nil
				})
			}
			return nil
		})
	}

	fmt.Printf("-- %d metric bytes\n", w.Len()-lenHeader)

	return nil
}

/*
flat row format, with strings in a dictionary, and separate dict for tags

-- 267059 bytes in 18284 strings
-- 55215 tagsets
raw = 11415500, t1 = 0.583174963
gz1 = 4241228, t2 = 0.120945953
*/
func (b *bucket) serialize03(w *bytes.Buffer) error {
	i := newInterner()

	type record struct {
		name   string
		tags   string
		kind   int
		value  float64
		sketch sketch
	}

	recs := []record{}
	for k1, l1 := range b.gauges {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: gaugeKind, value: val})
		}
	}
	for k1, l1 := range b.counts {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: countKind, value: val})
		}
	}
	for k1, l1 := range b.sketches {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: sketchKind, sketch: val})
		}
	}

	slices.SortFunc(recs, func(a, b record) int {
		if a.name < b.name {
			return -1
		}
		if a.name > b.name {
			return 1
		}
		if a.tags < b.tags {
			return -1
		}
		if a.tags > b.tags {
			return 1
		}
		return 0
	})

	for _, r := range recs {
		i.intern(r.name)
		i.internTags2(r.tags)
	}

	ps := molecule.NewProtoStream(w)
	ps.String(2, b.containerId)
	ps.Embedded(4, i.serialize)
	for _, r := range recs {
		ps.Embedded(3, func(ps *molecule.ProtoStream) error {
			ps.Int64(1, i.intern(r.name))
			ps.Int64(2, i.internTags2(r.tags))
			switch r.kind {
			case gaugeKind:
				ps.Double(4, r.value)
			case countKind:
				ps.Double(5, r.value)
			case sketchKind:
				ps.Embedded(6+int(r.sketch.mode), func(ps *molecule.ProtoStream) error {
					sk := r.sketch.sk.Finish()
					b := sk.Basic
					k, n := sk.Cols()
					ps.Int64(2, b.Cnt)
					ps.Double(3, b.Min)
					ps.Double(4, b.Max)
					ps.Double(5, b.Avg)
					ps.Double(6, b.Sum)
					ps.Sint32Packed(7, k)
					ps.Uint32Packed(8, n)
					return nil
				})
			}
			return nil
		})
	}

	return nil
}

/*
flat row format, with strings in a dictionary, and separate dict for tags, with delta encoding for sketch columns
(serialize3 +delta encoding for sketch columns)

-- 267059 bytes in 18284 strings
-- 55215 tagsets
-- 1202187 header bytes
-- 9859222 metric bytes
raw = 11061409, t1 = 0.640722506
gz1 = 4119335, t2 = 0.119286119
*/
func (b *bucket) serialize04(w *bytes.Buffer) error {
	i := newInterner()

	type record struct {
		name   string
		tags   string
		kind   int
		value  float64
		sketch sketch
	}

	recs := []record{}
	for k1, l1 := range b.gauges {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: gaugeKind, value: val})
		}
	}
	for k1, l1 := range b.counts {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: countKind, value: val})
		}
	}
	for k1, l1 := range b.sketches {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: sketchKind, sketch: val})
		}
	}

	slices.SortFunc(recs, func(a, b record) int {
		if a.name < b.name {
			return -1
		}
		if a.name > b.name {
			return 1
		}
		if a.tags < b.tags {
			return -1
		}
		if a.tags > b.tags {
			return 1
		}
		return 0
	})

	for _, r := range recs {
		i.intern(r.name)
		i.internTags2(r.tags)
	}

	ps := molecule.NewProtoStream(w)
	ps.String(2, b.containerId)
	ps.Embedded(4, i.serialize)

	lenHeader := w.Len()
	fmt.Printf("-- %d header bytes\n", lenHeader)

	for _, r := range recs {
		ps.Embedded(3, func(ps *molecule.ProtoStream) error {
			ps.Int64(1, i.intern(r.name))
			ps.Int64(2, i.internTags2(r.tags))
			switch r.kind {
			case gaugeKind:
				ps.Double(4, r.value)
			case countKind:
				ps.Double(5, r.value)
			case sketchKind:
				ps.Embedded(6+int(r.sketch.mode), func(ps *molecule.ProtoStream) error {
					sk := r.sketch.sk.Finish()
					b := sk.Basic
					k, n := sk.Cols()
					deltaEncode(k)
					ps.Int64(2, b.Cnt)
					ps.Double(3, b.Min)
					ps.Double(4, b.Max)
					ps.Double(5, b.Avg)
					ps.Double(6, b.Sum)
					ps.Sint32Packed(7, k)
					ps.Uint32Packed(8, n)
					return nil
				})
			}
			return nil
		})
	}

	fmt.Printf("-- %d metric bytes\n", w.Len()-lenHeader)

	return nil
}

/*
row based, with string and tagset dicts, delta encoding for sketch columns, separate global tags
(serialize4 + global tags)

-- 266901 bytes in 18277 strings
-- 55215 tagsets
-- 7 global tags
-- 981351 header bytes
-- 9859222 metric bytes
raw = 10840573, t1 = 0.626636042
gz1 = 4113882, t2 = 0.126338143
*/
func (b *bucket) serialize06(w *bytes.Buffer) error {
	i := newInterner()

	type record struct {
		name   string
		tags   string
		kind   int
		value  float64
		sketch sketch
	}

	recs := []record{}
	for k1, l1 := range b.gauges {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: gaugeKind, value: val})
		}
	}
	for k1, l1 := range b.counts {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: countKind, value: val})
		}
	}
	for k1, l1 := range b.sketches {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: sketchKind, sketch: val})
		}
	}

	slices.SortFunc(recs, func(a, b record) int {
		if a.name < b.name {
			return -1
		}
		if a.name > b.name {
			return 1
		}
		if a.tags < b.tags {
			return -1
		}
		if a.tags > b.tags {
			return 1
		}
		return 0
	})

	for _, r := range recs {
		i.intern(r.name)
		i.internTags3(r.tags)
	}

	ps := molecule.NewProtoStream(w)
	ps.String(2, b.containerId)
	ps.Embedded(4, i.serialize)

	lenHeader := w.Len()
	fmt.Printf("-- %d header bytes\n", lenHeader)

	for _, r := range recs {
		ps.Embedded(3, func(ps *molecule.ProtoStream) error {
			ps.Int64(1, i.intern(r.name))
			ps.Int64(2, i.internTags3(r.tags))
			switch r.kind {
			case gaugeKind:
				ps.Double(4, r.value)
			case countKind:
				ps.Double(5, r.value)
			case sketchKind:
				ps.Embedded(6+int(r.sketch.mode), func(ps *molecule.ProtoStream) error {
					sk := r.sketch.sk.Finish()
					b := sk.Basic
					k, n := sk.Cols()
					deltaEncode(k)
					ps.Int64(2, b.Cnt)
					ps.Double(3, b.Min)
					ps.Double(4, b.Max)
					ps.Double(5, b.Avg)
					ps.Double(6, b.Sum)
					ps.Sint32Packed(7, k)
					ps.Uint32Packed(8, n)
					return nil
				})
			}
			return nil
		})
	}

	fmt.Printf("-- %d metric bytes\n", w.Len()-lenHeader)

	return nil
}

func extend(is []uint32) []int64 {
	r := make([]int64, 0, len(is))
	for _, i := range is {
		r = append(r, int64(i))
	}
	return r
}

/*
columnar, with string and tagset dicts, delta encoding, separate global tags

-- 189085 metrics
-- 266901 bytes in 18277 strings
-- 55215 tagsets
-- 7 global tags
-- 981351 header bytes
-- 7255060 metric bytes
raw = 8236411, t1 = 0.83909932
gz1 = 2587605, t2 = 0.24713141
*/
func (b *bucket) serialize07(w *bytes.Buffer) error {
	i := newInterner()

	type record struct {
		name   string
		tags   string
		kind   int
		value  float64
		sketch sketch
	}

	recs := []record{}
	for k1, l1 := range b.gauges {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: gaugeKind, value: val})
		}
	}
	for k1, l1 := range b.counts {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: countKind, value: val})
		}
	}
	for k1, l1 := range b.sketches {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: sketchKind, sketch: val})
		}
	}

	slices.SortFunc(recs, func(a, b record) int {
		if a.name < b.name {
			return -1
		}
		if a.name > b.name {
			return 1
		}
		if a.tags < b.tags {
			return -1
		}
		if a.tags > b.tags {
			return 1
		}
		return 0
	})

	names := []int64{}
	tags := []int64{}
	types := []int64{}
	floats := []float64{}
	skcnts := []int64{}
	sklens := []int64{}
	skks := []int32{}
	skns := []uint32{}

	for _, r := range recs {
		names = append(names, i.intern(r.name))
		tags = append(tags, i.internTags3(r.tags))
		types = append(types, int64(r.kind)+int64(r.sketch.mode))
		switch r.kind {
		case gaugeKind, countKind:
			floats = append(floats, r.value)
		case sketchKind:
			sk := r.sketch.sk.Finish()
			b := sk.Basic
			k, n := sk.Cols()
			deltaEncode(k)
			floats = append(floats, b.Min, b.Max, b.Avg, b.Sum)
			skcnts = append(skcnts, b.Cnt)
			sklens = append(sklens, int64(len(k)))
			skks = append(skks, k...)
			skns = append(skns, n...)
		}
	}

	deltaEncode(names)
	deltaEncode(tags)

	fmt.Printf("-- %d metrics\n", len(names))

	ps := molecule.NewProtoStream(w)
	ps.String(2, b.containerId)
	ps.Embedded(4, i.serialize)

	lenHeader := w.Len()
	fmt.Printf("-- %d header bytes\n", lenHeader)

	ps.Int64(5, int64(len(names)))
	ps.Int64Packed(10, names)
	ps.Int64Packed(11, tags)
	ps.Int64Packed(12, types)
	ps.DoublePacked(13, floats)
	ps.Int64Packed(14, skcnts)
	ps.Int64Packed(15, sklens)
	ps.Sint32Packed(16, skks)
	ps.Uint32Packed(17, skns)

	fmt.Printf("-- %d metric bytes\n", w.Len()-lenHeader)

	return nil
}

/*
columnar, with string and tagset dicts, delta encoding, separate global tags, group metrics together by type first
(serialize7 + group metrics together by type first)

-- 189085 metrics
-- 6376 zero floats
-- 266901 bytes in 18277 strings
-- 55215 tagsets
-- 7 global tags
-- 1001705 header bytes
-- 189095 bytes in metric name pointers
-- 7065982 metric bytes
raw = 8067687, t1 = 0.731384195
gz1 = 2578803, t2 = 0.077460128
*/
func (b *bucket) serialize08(w *bytes.Buffer) error {
	i := newInterner()

	type record struct {
		name   string
		tags   string
		kind   int
		value  float64
		sketch sketch
	}

	recs := []record{}
	for k1, l1 := range b.gauges {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: gaugeKind, value: val})
		}
	}
	for k1, l1 := range b.counts {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: countKind, value: val})
		}
	}
	for k1, l1 := range b.sketches {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: sketchKind, sketch: val})
		}
	}

	slices.SortFunc(recs, func(a, b record) int {
		if a.kind < b.kind {
			return -1
		}
		if a.kind > b.kind {
			return 1
		}
		if a.name < b.name {
			return -1
		}
		if a.name > b.name {
			return 1
		}
		if a.tags < b.tags {
			return -1
		}
		if a.tags > b.tags {
			return 1
		}
		return 0
	})

	zeroFloats := 0
	names := []int64{}
	tags := []int64{}
	types := make([]int64, 4)
	floats := []float64{}
	skcnts := []int64{}
	sklens := []int64{}
	skks := []int32{}
	skns := []uint32{}

	for _, r := range recs {
		names = append(names, i.intern(r.name))
		tags = append(tags, i.internTags3(r.tags))
		types[r.kind+int(r.sketch.mode)]++
		switch r.kind {
		case gaugeKind, countKind:
			zeroFloats += iszero(r.value)
			floats = append(floats, r.value)
		case sketchKind:
			sk := r.sketch.sk.Finish()
			b := sk.Basic
			k, n := sk.Cols()
			deltaEncode(k)
			floats = append(floats, b.Min, b.Max, b.Avg, b.Sum)
			zeroFloats += iszero(b.Min) + iszero(b.Max) + iszero(b.Avg) + iszero(b.Sum)
			skcnts = append(skcnts, b.Cnt)
			sklens = append(sklens, int64(len(k)))
			skks = append(skks, k...)
			skns = append(skns, n...)
		}
	}

	deltaEncode(names)
	deltaEncode(tags)
	fmt.Printf("-- %d metrics\n", len(names))
	// on some data, non-columnar format wins because protobuf can skip zero double values
	fmt.Printf("-- %d zero floats\n", zeroFloats)

	ps := molecule.NewProtoStream(w)
	ps.String(2, b.containerId)
	ps.Embedded(4, i.serialize)
	ps.Int64(5, int64(len(names)))

	lenHeader := w.Len()
	fmt.Printf("-- %d header bytes\n", lenHeader)

	ps.Int64Packed(10, names)

	fmt.Printf("-- %d bytes in metric name pointers\n", w.Len()-lenHeader)

	ps.Int64Packed(11, tags)
	ps.Int64Packed(12, types)
	ps.DoublePacked(13, floats)
	ps.Int64Packed(14, skcnts)
	ps.Int64Packed(15, sklens)
	ps.Sint32Packed(16, skks)
	ps.Uint32Packed(17, skns)

	fmt.Printf("-- %d metric bytes\n", w.Len()-lenHeader)

	return nil
}

type streamInterner struct {
	idx map[string]int64
	str []string
	top int64
	lim int64

	global map[string]struct{}

	tags    map[string]int64
	tagsTop int64

	repeatNames int
	repeatTags  int
	reusedTags  int

	maxBackrefName int64
	maxBackrefTags int64
}

func newStreamInterner(lim int64) *streamInterner {
	return &streamInterner{
		idx: make(map[string]int64),
		lim: lim,

		global: make(map[string]struct{}),
		tags:   make(map[string]int64),
	}
}

func (si *streamInterner) intern(s string) int64 {
	if k, ok := si.idx[s]; ok {
		ref := si.top - k
		if ref < si.lim {
			if ref > si.maxBackrefName {
				si.maxBackrefName = ref
			}
			return ref
		}
		si.repeatNames++
		delete(si.idx, s)
	}
	si.idx[s] = si.top
	si.str = append(si.str, s)
	si.top++
	return -int64(len(s))
}

func (si *streamInterner) internTags(s string) (int64, []int64) {
	if k, ok := si.tags[s]; ok {
		ref := si.tagsTop - k
		if ref < si.lim {
			if ref > si.maxBackrefTags {
				si.maxBackrefTags = ref
			}
			si.reusedTags++
			return ref, nil
		}
		si.repeatTags++
		delete(si.tags, s)
	}

	is := []int64{}
	for _, s := range strings.Split(s, ",") {
		pair := strings.SplitN(s, ":", 2)
		if _, ok := globalTags[pair[0]]; ok {
			si.global[s] = struct{}{}
		} else {
			k := si.intern(s)
			is = append(is, k)
		}
	}

	si.tags[s] = si.tagsTop
	si.tagsTop++

	return -int64(len(is)), is
}

func (si *streamInterner) intern2(s string, w *tableStream) error {
	if k, ok := si.idx[s]; ok {
		ref := si.top - k
		if ref < si.lim {
			if ref > si.maxBackrefName {
				si.maxBackrefName = ref
			}
			return w.sint64(ref)
		}
		si.repeatNames++
		delete(si.idx, s)
	}
	si.idx[s] = si.top
	si.str = append(si.str, s)
	si.top++
	err := w.sint64(-int64(len(s)))
	if err != nil {
		return err
	}
	return w.bytes([]byte(s))
}

func (si *streamInterner) internTags2(s string, w *tableStream) error {
	if k, ok := si.tags[s]; ok {
		ref := si.tagsTop - k
		if ref < si.lim {
			if ref > si.maxBackrefTags {
				si.maxBackrefTags = ref
			}
			si.reusedTags++
			return w.sint64(ref)
		}
		si.repeatTags++
		delete(si.tags, s)
	}

	ts := []string{}
	for _, s := range strings.Split(s, ",") {
		pair := strings.SplitN(s, ":", 2)
		if _, ok := globalTags[pair[0]]; ok {
			si.global[s] = struct{}{}
		} else {
			ts = append(ts, s)
		}
	}

	err := w.sint64(-int64(len(ts)))
	if err != nil {
		return err
	}

	for _, s := range ts {
		err = si.intern2(s, w)
		if err != nil {
			return err
		}
	}

	si.tags[s] = si.tagsTop
	si.tagsTop++

	return nil
}

/*
columnar, with delta encoding for sketch columns, with streaming interning

-- 189085 metrics
-- 6376 zero floats
-- 164 header bytes
-- 189093 bytes in metric name pointers
-- 914672 bytes in tagsets (440565 tags, 189085 tagsets)
-- 7573962 metric bytes
-- 699595 strings bytes
-- 0 repeat names
-- 33389 repeat tag strings, 0 repeat tagsets, 133870 reused tagsets
raw = 8462814, t1 = 0.75496221
gz1 = 1992182, t2 = 0.064854761
*/
func (b *bucket) serialize09(w *bytes.Buffer) error {
	type record struct {
		name   string
		tags   string
		kind   int
		value  float64
		sketch sketch
	}

	recs := []record{}
	for k1, l1 := range b.gauges {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: gaugeKind, value: val})
		}
	}
	for k1, l1 := range b.counts {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: countKind, value: val})
		}
	}
	for k1, l1 := range b.sketches {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: sketchKind, sketch: val})
		}
	}

	slices.SortFunc(recs, func(a, b record) int {
		if a.tags < b.tags {
			return -1
		}
		if a.tags > b.tags {
			return 1
		}
		if a.name < b.name {
			return -1
		}
		if a.name > b.name {
			return 1
		}
		return 0
	})

	zeroFloats := 0
	names := []int64{}
	tagsets := []int64{}
	tags := []int64{}
	types := make([]int64, 4)
	floats := []float64{}
	skcnts := []int64{}
	sklens := []int64{}
	skks := []int32{}
	skns := []uint32{}

	sn := newStreamInterner(256)
	st := newStreamInterner(256)

	for _, r := range recs {
		nameidx := sn.intern(r.name)
		names = append(names, nameidx)

		tagset, taglist := st.internTags(r.tags)
		tagsets = append(tagsets, tagset)
		tags = append(tags, taglist...)

		types[r.kind+int(r.sketch.mode)]++
		switch r.kind {
		case gaugeKind, countKind:
			zeroFloats += iszero(r.value)
			floats = append(floats, r.value)
		case sketchKind:
			sk := r.sketch.sk.Finish()
			b := sk.Basic
			k, n := sk.Cols()
			deltaEncode(k)
			floats = append(floats, b.Min, b.Max, b.Avg, b.Sum)
			zeroFloats += iszero(b.Min) + iszero(b.Max) + iszero(b.Avg) + iszero(b.Sum)
			skcnts = append(skcnts, b.Cnt)
			sklens = append(sklens, int64(len(k)))
			skks = append(skks, k...)
			skns = append(skns, n...)
		}
	}

	fmt.Printf("-- %d metrics\n", len(names))
	// on some data, non-columnar format wins because protobuf skips zero values
	fmt.Printf("-- %d zero floats\n", zeroFloats)

	ps := molecule.NewProtoStream(w)
	prevlen := 0
	ps.String(2, b.containerId)
	ps.String(6, strings.Join(slices.Collect(maps.Keys(st.global)), "\x00"))
	ps.Int64(5, int64(len(names)))

	fmt.Printf("-- %d header bytes\n", w.Len()-prevlen)
	prevlen = w.Len()

	ps.Sint64Packed(10, names)

	fmt.Printf("-- %d bytes in metric name pointers\n", w.Len()-prevlen)
	prevlen = w.Len()

	ps.Sint64Packed(18, tagsets)
	ps.Sint64Packed(11, tags)

	fmt.Printf("-- %d bytes in tagsets (%d tags, %d tagsets)\n", w.Len()-prevlen, len(tags), len(tagsets))

	ps.Int64Packed(12, types)
	ps.DoublePacked(13, floats)
	ps.Int64Packed(14, skcnts)
	ps.Int64Packed(15, sklens)
	ps.Sint32Packed(16, skks)
	ps.Uint32Packed(17, skns)

	fmt.Printf("-- %d metric bytes\n", w.Len()-prevlen)
	prevlen = w.Len()

	ps.String(21, strings.Join(sn.str, ""))
	ps.String(22, strings.Join(st.str, ""))

	fmt.Printf("-- %d strings bytes\n", w.Len()-prevlen)
	fmt.Printf("-- %d repeat names\n", sn.repeatNames)
	fmt.Printf("-- %d repeat tag strings, %d repeat tagsets, %d reused tagsets\n", st.repeatNames, st.repeatTags, st.reusedTags)

	return nil
}

type tableStream struct {
	w   *bytes.Buffer
	buf [16]byte
}

func (s *tableStream) uint64(u uint64) error {
	n := binary.PutUvarint(s.buf[:], u)
	_, err := s.w.Write(s.buf[:n])
	return err
}

func (s *tableStream) sint64(v int64) error {
	u := uint64(v<<1) ^ uint64(v>>63)
	return s.uint64(u)
}

func (s *tableStream) bytes(b []byte) error {
	_, err := s.w.Write(b)
	return err
}

func (s *tableStream) double(v float64) error {
	n, err := binary.Encode(s.buf[:], binary.LittleEndian, v)
	if err != nil {
		return err
	}
	_, err = s.w.Write(s.buf[:n])
	return err
}

func (s *tableStream) float(v float32) error {
	n, err := binary.Encode(s.buf[:], binary.LittleEndian, v)
	if err != nil {
		return err
	}
	if n != 4 {
		panic(n)
	}
	n, err = s.w.Write(s.buf[:n])
	if n != 4 {
		panic(n)
	}
	return err
}

func (s *tableStream) pbBytes(fid uint64, bytes []byte) error {
	err := s.uint64(fid<<3 | 2)
	if err != nil {
		return err
	}
	err = s.uint64(uint64(len(bytes)))
	if err != nil {
		return err
	}
	return s.bytes(bytes)
}

/*
row based, without field identifiers, inline interning

-- 164 header bytes
-- 8870143 total bytes
-- 0 repeat names
-- 33389 repeat tag strings, 0 repeat tagsets, 133870 reused tagsets
raw = 8870307, t1 = 0.596837435
gz1 = 2898357, t2 = 0.091003303
*/
func (b *bucket) serialize10(w *bytes.Buffer) error {
	type record struct {
		name   string
		tags   string
		kind   int
		value  float64
		sketch sketch
	}

	recs := []record{}
	for k1, l1 := range b.gauges {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: gaugeKind, value: val})
		}
	}
	for k1, l1 := range b.counts {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: countKind, value: val})
		}
	}
	for k1, l1 := range b.sketches {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: sketchKind, sketch: val})
		}
	}

	slices.SortFunc(recs, func(a, b record) int {
		if a.tags < b.tags {
			return -1
		}
		if a.tags > b.tags {
			return 1
		}
		if a.name < b.name {
			return -1
		}
		if a.name > b.name {
			return 1
		}
		return 0
	})

	sn := newStreamInterner(256)
	st := newStreamInterner(256)

	outer := &tableStream{w: bytes.NewBuffer(nil)}
	ts := &tableStream{w: bytes.NewBuffer(nil)}

	for _, r := range recs {
		ts.w.Reset()
		sn.intern2(r.name, ts)
		st.internTags2(r.tags, ts)
		switch r.kind {
		case gaugeKind, countKind:
			ts.sint64(int64(r.kind))
			ts.double(r.value)
		case sketchKind:
			sk := r.sketch.sk.Finish()
			b := sk.Basic
			k, n := sk.Cols()
			deltaEncode(k)
			ts.sint64(int64(r.kind) + int64(r.sketch.mode))
			ts.double(b.Min)
			ts.double(b.Max)
			ts.double(b.Sum)
			ts.double(b.Avg)
			ts.sint64(b.Cnt)
			ts.sint64(int64(len(k)))
			for _, v := range k {
				ts.sint64(int64(v))
			}
			for _, v := range n {
				ts.uint64(uint64(v))
			}
		}
		outer.sint64(int64(ts.w.Len()))
		outer.bytes(ts.w.Bytes())
	}

	ps := molecule.NewProtoStream(w)
	ps.String(2, b.containerId)
	ps.Int64(5, int64(len(recs)))
	ps.String(6, strings.Join(slices.Collect(maps.Keys(st.global)), "\x00"))
	fmt.Printf("-- %d header bytes\n", w.Len())
	prevlen := w.Len()
	ps.Bytes(7, outer.w.Bytes())

	fmt.Printf("-- %d total bytes\n", w.Len()-prevlen)
	fmt.Printf("-- %d repeat names\n", sn.repeatNames)
	fmt.Printf("-- %d repeat tag strings, %d repeat tagsets, %d reused tagsets\n", st.repeatNames, st.repeatTags, st.reusedTags)

	return nil
}

const (
	typeZero   = 0x00
	typeInt    = 0x10
	typeFloat  = 0x20
	typeDouble = 0x30
)

/*
row based, without field identifiers, inline interning, and compacted values when possible

-- 164 header bytes
-- 5640301 total bytes
-- 0 repeat names
-- 33389 repeat tag strings, 0 repeat tagsets, 133870 reused tagsets
-- type counts: 6017 zero, 156408 int8, 0 float, 26660 double
-- maximum backref names: 11, tag name: 255, tagset: 1
raw = 5640465, t1 = 0.633120757
gz1 = 2354224, t2 = 0.069427609
*/
func (b *bucket) serialize11(w *bytes.Buffer) error {
	type record struct {
		name   string
		tags   string
		kind   int
		value  float64
		sketch sketch
	}

	recs := []record{}
	for k1, l1 := range b.gauges {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: gaugeKind, value: val})
		}
	}
	for k1, l1 := range b.counts {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: countKind, value: val})
		}
	}
	for k1, l1 := range b.sketches {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: sketchKind, sketch: val})
		}
	}

	slices.SortFunc(recs, func(a, b record) int {
		if a.tags < b.tags {
			return -1
		}
		if a.tags > b.tags {
			return 1
		}
		if a.name < b.name {
			return -1
		}
		if a.name > b.name {
			return 1
		}
		return 0
	})

	sn := newStreamInterner(256)
	st := newStreamInterner(256)

	outer := &tableStream{w: bytes.NewBuffer(nil)}
	ts := &tableStream{w: bytes.NewBuffer(nil)}

	var zeroValues, int8Values, floatValues, doubleValues int
	for _, r := range recs {
		ts.w.Reset()
		sn.intern2(r.name, ts)
		st.internTags2(r.tags, ts)
		switch r.kind {
		case gaugeKind, countKind:
			if r.value == 0 {
				zeroValues++
				ts.uint64(uint64(r.kind) | typeZero)
			} else if isint8(r.value) {
				int8Values++
				ts.uint64(uint64(r.kind) | typeInt)
				ts.sint64(int64(r.value))
			} else if isfloat(r.value) {
				floatValues++
				ts.uint64(uint64(r.kind) | typeFloat)
				ts.float(float32(r.value))
			} else {
				doubleValues++
				ts.uint64(uint64(r.kind) | typeDouble)
				ts.double(r.value)
			}
		case sketchKind:
			sk := r.sketch.sk.Finish()
			b := sk.Basic
			k, n := sk.Cols()
			deltaEncode(k)
			ty := uint64(r.kind) + uint64(r.sketch.mode)
			if b.Min == 0 && b.Max == 0 && b.Sum == 0 && b.Cnt == 0 {
				zeroValues++
				ts.uint64(ty | typeZero)
			} else if isint8(b.Min) && isint8(b.Max) && isint8(b.Sum) {
				int8Values++
				ts.uint64(ty | typeInt)
				ts.sint64(int64(b.Min))
				ts.sint64(int64(b.Max))
				ts.sint64(int64(b.Sum))
			} else if isfloat(b.Min) && isfloat(b.Max) && isfloat(b.Sum) {
				floatValues++
				ts.uint64(ty | typeFloat)
				ts.float(float32(b.Min))
				ts.float(float32(b.Max))
				ts.float(float32(b.Sum))
			} else {
				doubleValues++
				ts.uint64(ty | typeDouble)
				ts.double(b.Min)
				ts.double(b.Max)
				ts.double(b.Sum)
			}
			//ts.double(b.Avg)
			ts.sint64(b.Cnt)
			ts.sint64(int64(len(k)))
			for _, v := range k {
				ts.sint64(int64(v))
			}
			for _, v := range n {
				ts.uint64(uint64(v))
			}
		}

		outer.uint64(uint64(ts.w.Len()))
		outer.bytes(ts.w.Bytes())
	}

	ps := molecule.NewProtoStream(w)
	ps.String(2, b.containerId)
	ps.Int64(5, int64(len(recs)))
	ps.String(6, strings.Join(slices.Collect(maps.Keys(st.global)), "\x00"))
	fmt.Printf("-- %d header bytes\n", w.Len())
	prevlen := w.Len()

	ps.Bytes(7, outer.w.Bytes())

	fmt.Printf("-- %d total bytes\n", w.Len()-prevlen)
	fmt.Printf("-- %d repeat names\n", sn.repeatNames)
	fmt.Printf("-- %d repeat tag strings, %d repeat tagsets, %d reused tagsets\n", st.repeatNames, st.repeatTags, st.reusedTags)
	fmt.Printf("-- type counts: %d zero, %d int8, %d float, %d double\n", zeroValues, int8Values, floatValues, doubleValues)
	fmt.Printf("-- maximum backref names: %d, tag name: %d, tagset: %d\n", sn.maxBackrefName, st.maxBackrefName, st.maxBackrefTags)

	return nil
}

/*
tree-like, with tagsets as inner nodes and metric data as leafs, allowing nested tagsets

-- 164 header bytes
-- 5850840 total bytes
-- 0 repeat names
-- 33389 repeat tag strings, 0 repeat tagsets, 0 reused tagsets
-- type counts: 6017 zero, 156408 int8, 0 float, 26660 double
-- maximum backref names: 11, tag name: 255, tagset: 0
raw = 5851004, t1 = 0.57931239
gz1 = 2393638, t2 = 0.073600222
*/
func (b *bucket) serialize12(w *bytes.Buffer) error {
	type record struct {
		name   string
		tags   string
		kind   int
		value  float64
		sketch sketch
	}

	recs := []record{}
	for k1, l1 := range b.gauges {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: gaugeKind, value: val})
		}
	}
	for k1, l1 := range b.counts {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: countKind, value: val})
		}
	}
	for k1, l1 := range b.sketches {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: sketchKind, sketch: val})
		}
	}

	slices.SortFunc(recs, func(a, b record) int {
		if a.tags < b.tags {
			return -1
		}
		if a.tags > b.tags {
			return 1
		}
		if a.name < b.name {
			return -1
		}
		if a.name > b.name {
			return 1
		}
		return 0
	})

	sn := newStreamInterner(256)
	st := newStreamInterner(256)

	/*
	   message Metrics {
	     string containerId = 2;
	     Stream stream = 7;
	   }

	   message Stream {
	     repeated Group group = 1;
	   }

	   message Group {
	     bytes tags = 1;
	     oneof content {
	       bytes metrics = 2;
	       Group subgroup = 3;
	     }
	   }
	*/

	// 7: { outer
	//    1: { group
	//      1: { tags bytes }
	//      2: { metrics bytes }
	//    }
	//    (group repeats)
	// }

	outer := &tableStream{w: bytes.NewBuffer(nil)}
	group := &tableStream{w: bytes.NewBuffer(nil)}
	tags := &tableStream{w: bytes.NewBuffer(nil)}
	metrics := &tableStream{w: bytes.NewBuffer(nil)}
	ts := &tableStream{w: bytes.NewBuffer(nil)}

	var zeroValues, int8Values, floatValues, doubleValues int

	// tags for the current group
	var currentTags string

	for _, r := range recs {
		if currentTags != r.tags {
			if metrics.w.Len() > 0 {
				group.w.Reset()
				tags.w.Reset()
				st.internTags2(currentTags, tags)
				group.pbBytes(1, tags.w.Bytes())
				group.pbBytes(2, metrics.w.Bytes())
				outer.pbBytes(1, group.w.Bytes())
				metrics.w.Reset()
			}
			currentTags = r.tags
		}

		ts.w.Reset()
		sn.intern2(r.name, ts)

		switch r.kind {
		case gaugeKind, countKind:
			if r.value == 0 {
				zeroValues++
				ts.uint64(uint64(r.kind) | typeZero)
			} else if isint8(r.value) {
				int8Values++
				ts.uint64(uint64(r.kind) | typeInt)
				ts.sint64(int64(r.value))
			} else if isfloat(r.value) {
				floatValues++
				ts.uint64(uint64(r.kind) | typeFloat)
				ts.float(float32(r.value))
			} else {
				doubleValues++
				ts.uint64(uint64(r.kind) | typeDouble)
				ts.double(r.value)
			}
		case sketchKind:
			sk := r.sketch.sk.Finish()
			b := sk.Basic
			k, n := sk.Cols()
			deltaEncode(k)
			ty := uint64(r.kind) + uint64(r.sketch.mode)
			if b.Min == 0 && b.Max == 0 && b.Sum == 0 && b.Cnt == 0 {
				zeroValues++
				ts.uint64(ty | typeZero)
			} else if isint8(b.Min) && isint8(b.Max) && isint8(b.Sum) {
				int8Values++
				ts.uint64(ty | typeInt)
				ts.sint64(int64(b.Min))
				ts.sint64(int64(b.Max))
				ts.sint64(int64(b.Sum))
			} else if isfloat(b.Min) && isfloat(b.Max) && isfloat(b.Sum) {
				floatValues++
				ts.uint64(ty | typeFloat)
				ts.float(float32(b.Min))
				ts.float(float32(b.Max))
				ts.float(float32(b.Sum))
			} else {
				doubleValues++
				ts.uint64(ty | typeDouble)
				ts.double(b.Min)
				ts.double(b.Max)
				ts.double(b.Sum)
			}
			//ts.double(b.Avg)
			ts.sint64(b.Cnt)
			ts.sint64(int64(len(k)))
			for _, v := range k {
				ts.sint64(int64(v))
			}
			for _, v := range n {
				ts.uint64(uint64(v))
			}
		}

		metrics.uint64(uint64(ts.w.Len()))
		metrics.bytes(ts.w.Bytes())
	}

	if metrics.w.Len() > 0 {
		group.w.Reset()
		tags.w.Reset()
		st.internTags2(currentTags, tags)
		group.pbBytes(1, tags.w.Bytes())
		group.pbBytes(2, metrics.w.Bytes())
		outer.pbBytes(1, group.w.Bytes())
	}

	ps := molecule.NewProtoStream(w)
	ps.String(2, b.containerId)
	ps.Int64(5, int64(len(recs)))
	ps.String(6, strings.Join(slices.Collect(maps.Keys(st.global)), "\x00"))
	fmt.Printf("-- %d header bytes\n", w.Len())
	prevlen := w.Len()

	ps.Bytes(7, outer.w.Bytes())

	fmt.Printf("-- %d total bytes\n", w.Len()-prevlen)
	fmt.Printf("-- %d repeat names\n", sn.repeatNames)
	fmt.Printf("-- %d repeat tag strings, %d repeat tagsets, %d reused tagsets\n", st.repeatNames, st.repeatTags, st.reusedTags)
	fmt.Printf("-- type counts: %d zero, %d int8, %d float, %d double\n", zeroValues, int8Values, floatValues, doubleValues)
	fmt.Printf("-- maximum backref names: %d, tag name: %d, tagset: %d\n", sn.maxBackrefName, st.maxBackrefName, st.maxBackrefTags)

	return nil
}

/*
serialize07 minus tagset interning
(because 02 compresses better than 03 for some reason)
*/
func (b *bucket) serialize13(w *bytes.Buffer) error {
	i := newInterner()

	type record struct {
		name   string
		tags   string
		kind   int
		value  float64
		sketch sketch
	}

	recs := []record{}
	for k1, l1 := range b.gauges {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: gaugeKind, value: val})
		}
	}
	for k1, l1 := range b.counts {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: countKind, value: val})
		}
	}
	for k1, l1 := range b.sketches {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: sketchKind, sketch: val})
		}
	}

	slices.SortFunc(recs, func(a, b record) int {
		if a.name < b.name {
			return -1
		}
		if a.name > b.name {
			return 1
		}
		if a.tags < b.tags {
			return -1
		}
		if a.tags > b.tags {
			return 1
		}
		return 0
	})

	names := []int64{}
	tags := []int64{}
	tagslen := []int64{}
	types := []int64{}
	floats := []float64{}
	skcnts := []int64{}
	sklens := []int64{}
	skks := []int32{}
	skns := []uint32{}

	for _, r := range recs {
		names = append(names, i.intern(r.name))
		tt := i.internTags(r.tags)
		tags = append(tags, tt...)
		tagslen = append(tagslen, int64(len(tt)))
		types = append(types, int64(r.kind)+int64(r.sketch.mode))
		switch r.kind {
		case gaugeKind, countKind:
			floats = append(floats, r.value)
		case sketchKind:
			sk := r.sketch.sk.Finish()
			b := sk.Basic
			k, n := sk.Cols()
			deltaEncode(k)
			floats = append(floats, b.Min, b.Max, b.Avg, b.Sum)
			skcnts = append(skcnts, b.Cnt)
			sklens = append(sklens, int64(len(k)))
			skks = append(skks, k...)
			skns = append(skns, n...)
		}
	}

	deltaEncode(names)
	deltaEncode(tags)
	deltaEncode(tagslen)

	fmt.Printf("-- %d metrics\n", len(names))

	ps := molecule.NewProtoStream(w)
	ps.String(2, b.containerId)
	ps.Embedded(4, i.serialize)

	lenHeader := w.Len()
	fmt.Printf("-- %d header bytes\n", lenHeader)

	ps.Int64(5, int64(len(names)))
	ps.Sint64Packed(10, names)
	ps.Sint64Packed(11, tags)
	ps.Sint64Packed(18, tagslen)
	ps.Int64Packed(12, types)
	ps.DoublePacked(13, floats)
	ps.Int64Packed(14, skcnts)
	ps.Int64Packed(15, sklens)
	ps.Sint32Packed(16, skks)
	ps.Uint32Packed(17, skns)

	fmt.Printf("-- %d metric bytes\n", w.Len()-lenHeader)

	return nil
}

/*
serialize07 + compact valeus when possible
*/
func (b *bucket) serialize14(w *bytes.Buffer) error {
	i := newInterner()

	type record struct {
		name   string
		tags   string
		kind   int
		value  float64
		sketch sketch
	}

	recs := []record{}
	for k1, l1 := range b.gauges {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: gaugeKind, value: val})
		}
	}
	for k1, l1 := range b.counts {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: countKind, value: val})
		}
	}
	for k1, l1 := range b.sketches {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: sketchKind, sketch: val})
		}
	}

	slices.SortFunc(recs, func(a, b record) int {
		if a.name < b.name {
			return -1
		}
		if a.name > b.name {
			return 1
		}
		if a.tags < b.tags {
			return -1
		}
		if a.tags > b.tags {
			return 1
		}
		return 0
	})

	names := []int64{}
	tags := []int64{}
	types := []uint64{}
	int8s := []int64{}
	f32s := []float32{}
	f64s := []float64{}
	skcnts := []int64{}
	sklens := []int64{}
	skks := []int32{}
	skns := []uint32{}

	var zeroValues, int8Values, floatValues, doubleValues int

	for _, r := range recs {
		names = append(names, i.intern(r.name))
		tags = append(tags, i.internTags3(r.tags))
		switch r.kind {
		case gaugeKind, countKind:
			if r.value == 0 {
				zeroValues++
				types = append(types, uint64(r.kind)|typeZero)
			} else if isint8(r.value) {
				int8Values++
				types = append(types, uint64(r.kind)|typeInt)
				int8s = append(int8s, int64(r.value))
			} else if isfloat(r.value) {
				floatValues++
				types = append(types, uint64(r.kind)|typeFloat)
				f32s = append(f32s, float32(r.value))
			} else {
				doubleValues++
				types = append(types, uint64(r.kind)|typeDouble)
				f64s = append(f64s, r.value)
			}
		case sketchKind:
			sk := r.sketch.sk.Finish()
			b := sk.Basic
			k, n := sk.Cols()
			deltaEncode(k)
			ty := uint64(r.kind) + uint64(r.sketch.mode)
			if b.Min == 0 && b.Max == 0 && b.Sum == 0 && b.Cnt == 0 {
				zeroValues++
				types = append(types, ty|typeZero)
			} else if isint8(b.Min) && isint8(b.Max) && isint8(b.Sum) {
				int8Values++
				types = append(types, ty|typeInt)
				int8s = append(int8s, int64(b.Min), int64(b.Max), int64(b.Sum))
			} else if isfloat(b.Min) && isfloat(b.Max) && isfloat(b.Sum) {
				floatValues++
				types = append(types, ty|typeFloat)
				f32s = append(f32s, float32(b.Min), float32(b.Max), float32(b.Sum))
			} else {
				doubleValues++
				types = append(types, ty|typeDouble)
				f64s = append(f64s, b.Min, b.Max, b.Sum)
			}
			//ts.double(b.Avg)
			skcnts = append(skcnts, b.Cnt)
			sklens = append(sklens, int64(len(k)))
			skks = append(skks, k...)
			skns = append(skns, n...)
		}
	}

	deltaEncode(names)
	deltaEncode(tags)

	fmt.Printf("-- %d metrics\n", len(names))

	ps := molecule.NewProtoStream(w)
	ps.String(2, b.containerId)
	ps.Embedded(4, i.serialize)

	lenHeader := w.Len()
	fmt.Printf("-- %d header bytes\n", lenHeader)

	ps.Int64(5, int64(len(names)))
	ps.Int64Packed(10, names)
	ps.Int64Packed(11, tags)
	ps.Uint64Packed(12, types)
	ps.Int64Packed(14, skcnts)
	ps.Int64Packed(15, sklens)
	ps.Sint32Packed(16, skks)
	ps.Uint32Packed(17, skns)
	ps.Sint64Packed(18, int8s)
	ps.FloatPacked(19, f32s)
	ps.DoublePacked(20, f64s)

	fmt.Printf("-- %d metric bytes\n", w.Len()-lenHeader)
	fmt.Printf("-- type counts: %d zero, %d int8, %d float, %d double\n", zeroValues, int8Values, floatValues, doubleValues)

	return nil
}

/*
serialize07 + deinterleave f64
*/
func (b *bucket) serialize15(w *bytes.Buffer) error {
	i := newInterner()

	type record struct {
		name   string
		tags   string
		kind   int
		value  float64
		sketch sketch
	}

	recs := []record{}
	for k1, l1 := range b.gauges {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: gaugeKind, value: val})
		}
	}
	for k1, l1 := range b.counts {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: countKind, value: val})
		}
	}
	for k1, l1 := range b.sketches {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: sketchKind, sketch: val})
		}
	}

	slices.SortFunc(recs, func(a, b record) int {
		if a.name < b.name {
			return -1
		}
		if a.name > b.name {
			return 1
		}
		if a.tags < b.tags {
			return -1
		}
		if a.tags > b.tags {
			return 1
		}
		return 0
	})

	names := []int64{}
	tags := []int64{}
	types := []int64{}
	floats := []float64{}
	skcnts := []int64{}
	sklens := []int64{}
	skks := []int32{}
	skns := []uint32{}

	var zeroValues, int8Values, floatValues, doubleValues int

	for _, r := range recs {
		names = append(names, i.intern(r.name))
		tags = append(tags, i.internTags3(r.tags))

		types = append(types, int64(r.kind)+int64(r.sketch.mode))
		switch r.kind {
		case gaugeKind, countKind:
			floats = append(floats, r.value)
		case sketchKind:
			sk := r.sketch.sk.Finish()
			b := sk.Basic
			k, n := sk.Cols()
			deltaEncode(k)
			floats = append(floats, b.Min, b.Max, b.Avg, b.Sum)
			skcnts = append(skcnts, b.Cnt)
			sklens = append(sklens, int64(len(k)))
			skks = append(skks, k...)
			skns = append(skns, n...)
		}
	}

	deltaEncode(names)
	deltaEncode(tags)

	fmt.Printf("-- %d metrics\n", len(names))

	ps := molecule.NewProtoStream(w)
	ps.String(2, b.containerId)
	ps.Embedded(4, i.serialize)

	lenHeader := w.Len()
	fmt.Printf("-- %d header bytes\n", lenHeader)

	deinterleaved := [8][]byte{}
	for _, v := range floats {
		iv := math.Float64bits(v)
		for i := 0; i < 8; i++ {
			deinterleaved[i] = append(deinterleaved[i], byte(iv))
			iv >>= 8
		}
	}

	ps.Int64(5, int64(len(names)))
	ps.Int64Packed(10, names)
	ps.Int64Packed(11, tags)
	ps.Int64Packed(12, types)
	ps.Int64Packed(14, skcnts)
	ps.Int64Packed(15, sklens)
	ps.Sint32Packed(16, skks)
	ps.Uint32Packed(17, skns)

	for i := range deinterleaved {
		ps.Bytes(20+i, deinterleaved[i])
	}

	fmt.Printf("-- %d metric bytes\n", w.Len()-lenHeader)
	fmt.Printf("-- type counts: %d zero, %d int8, %d float, %d double\n", zeroValues, int8Values, floatValues, doubleValues)

	return nil
}

/*
like serialize07, don't pre-sort metrics, but pre-build and sort the dictionary.
*/
func (b *bucket) serialize16(w *bytes.Buffer, sort int) error {
	i := newInterner()

	type record struct {
		name   string
		tags   string
		kind   int
		value  float64
		sketch sketch
	}

	recs := []record{}
	for k1, l1 := range b.gauges {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: gaugeKind, value: val})
		}
	}
	for k1, l1 := range b.counts {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: countKind, value: val})
		}
	}
	for k1, l1 := range b.sketches {
		for k2, val := range l1 {
			recs = append(recs, record{name: k1, tags: k2, kind: sketchKind, sketch: val})
		}
	}

	if false {
		slices.SortFunc(recs, func(a, b record) int {
			if a.name < b.name {
				return -1
			}
			if a.name > b.name {
				return 1
			}
			if a.tags < b.tags {
				return -1
			}
			if a.tags > b.tags {
				return 1
			}
			return 0
		})
	}

	names := []int64{}
	tags := []int64{}
	types := []int64{}
	floats := []float64{}
	skcnts := []int64{}
	sklens := []int64{}
	skks := []int32{}
	skns := []uint32{}

	for _, r := range recs {
		i.intern(r.name)
		i.internTags(r.tags)
	}

	switch sort {
	case 0:
	case 1:
		i.sort()
	case 2:
		i.sortByFreq()
	}

	for _, r := range recs {
		names = append(names, i.intern(r.name))
		tags = append(tags, i.internTags3(r.tags))
		types = append(types, int64(r.kind)+int64(r.sketch.mode))
		switch r.kind {
		case gaugeKind, countKind:
			floats = append(floats, r.value)
		case sketchKind:
			sk := r.sketch.sk.Finish()
			b := sk.Basic
			k, n := sk.Cols()
			deltaEncode(k)
			floats = append(floats, b.Min, b.Max, b.Avg, b.Sum)
			skcnts = append(skcnts, b.Cnt)
			sklens = append(sklens, int64(len(k)))
			skks = append(skks, k...)
			skns = append(skns, n...)
		}
	}

	deltaEncode(names)
	deltaEncode(tags)

	fmt.Printf("-- %d metrics\n", len(names))

	ps := molecule.NewProtoStream(w)
	ps.String(2, b.containerId)
	ps.Embedded(4, i.serialize)

	lenHeader := w.Len()
	fmt.Printf("-- %d header bytes\n", lenHeader)

	ps.Int64(5, int64(len(names)))
	ps.Sint64Packed(10, names)
	ps.Sint64Packed(11, tags)
	ps.Int64Packed(12, types)
	ps.DoublePacked(13, floats)
	ps.Int64Packed(14, skcnts)
	ps.Int64Packed(15, sklens)
	ps.Sint32Packed(16, skks)
	ps.Uint32Packed(17, skns)

	fmt.Printf("-- %d metric bytes\n", w.Len()-lenHeader)

	return nil
}

func (b *bucket) serialize16a(w *bytes.Buffer) error {
	return b.serialize16(w, 0)
}
func (b *bucket) serialize16b(w *bytes.Buffer) error {
	return b.serialize16(w, 1)
}
func (b *bucket) serialize16c(w *bytes.Buffer) error {
	return b.serialize16(w, 2)
}

func iszero(v float64) int {
	if v == 0 {
		return 1
	}
	return 0
}

// is v an integer than can be varint encoded in 8 bytes or less
func isint8(v float64) bool {
	const max8 = 1 << 55
	const min8 = -1 << 55
	i := int64(v)
	return float64(i) == v && i < max8 && i >= min8
}

func isfloat(v float64) bool {
	return float64(float32(v)) == v
}

type agg struct {
	// ts -> pid -> bucket
	buckets map[int64]map[int]*bucket
}

func (a *agg) read(ts_ns int64, pid int, line string) {
	if a.buckets == nil {
		a.buckets = make(map[int64]map[int]*bucket)
	}

	for _, line := range strings.Split(line, "\n") {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "_sc|") || strings.HasPrefix(line, "_e|") {
			continue
		}
		fields := strings.Split(line, "|")
		values := strings.Split(fields[0], ":")
		tags := ""
		ts := ts_ns / 10_000_000_000
		containerId := ""

		for i := 2; i < len(fields); i++ {
			field := fields[i]
			if ts, ok := strings.CutPrefix(field, "#"); ok {
				list := strings.Split(ts, ",")
				list = sort.UniqInPlace(list)
				tags = strings.Join(list, ",")
			}
			if id, ok := strings.CutPrefix(field, "c:"); ok {
				containerId = id
			}
			// if tsstr, ok := strings.CutPrefix(field, "T"); ok {
			// 	continue payload
			// 	var err error
			// 	ts, err = strconv.ParseInt(tsstr, 10, 64)
			// 	if err != nil {
			// 		panic(err)
			// 	}
			// }
		}

		if strings.Contains(tags, "dd.internal.entity_id:none") || strings.Contains(tags, "client:rust") {
			continue
		}

		numvalues := make([]float64, 0, len(values)-1)
		for i := 1; i < len(values); i++ {
			v, err := strconv.ParseFloat(values[i], 64)
			if err != nil {
				fmt.Printf("parse err: %q: %v", values[i], err)
				continue
			}
			numvalues = append(numvalues, v)
		}

		ts = 0
		pm, ok := a.buckets[ts]
		if !ok {
			pm = make(map[int]*bucket)
			a.buckets[ts] = pm
		}
		b, ok := pm[pid]
		if !ok {
			b = &bucket{}
			pm[pid] = b
		}

		b.containerId = containerId
		b.insert(fields[1], values[0], tags, numvalues)
	}
}

var flagTs = flag.Int64("t", 0, "timestamp to use as a benchmark")
var flagPid = flag.Int("p", 0, "client pid")
var flagCap = flag.String("c", "capture.zstd", "dogstatsd capture file to read")

var flagProfCpu = flag.String("cpuprofile", "pprof.cpu", "where to write cpu profile to")
var flagProfMem = flag.String("memprofile", "pprof.mem", "where to write memory profile to")

func main() {
	flag.Parse()

	if *flagProfCpu != "" {
		f, err := os.Create(*flagProfCpu)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			panic(err)
		}
		defer pprof.StopCPUProfile()
	}

	cap, err := replay.NewTrafficCaptureReader(*flagCap, 16, false)
	if err != nil {
		panic(err)
	}
	a := agg{}
	cap.Init()
	for {
		msg, err := cap.ReadNext()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
		}
		a.read(msg.Timestamp, int(msg.Pid), string(msg.Payload))
	}

	fmt.Printf("available pids: %v\n", slices.Collect(maps.Keys(a.buckets[174773365])))

	funcs := []struct {
		name string
		fn   func(*bucket, *bytes.Buffer) error
	}{
		// {"serialize01", (*bucket).serialize01},
		// {"serialize02", (*bucket).serialize02},
		// {"serialize03", (*bucket).serialize03},
		// {"serialize04", (*bucket).serialize04},
		// {"serialize06", (*bucket).serialize06},
		{"serialize07", (*bucket).serialize07},
		// {"serialize08", (*bucket).serialize08},
		// {"serialize09", (*bucket).serialize09},
		// {"serialize10", (*bucket).serialize10},
		// {"serialize11", (*bucket).serialize11},
		// {"serialize12", (*bucket).serialize12},
		// {"serialize13", (*bucket).serialize13},
		// {"serialize14", (*bucket).serialize14},
		// {"serialize15", (*bucket).serialize15},
		{"serialize16_no", (*bucket).serialize16a},
		{"serialize16_lex", (*bucket).serialize16b},
		{"serialize16_frq", (*bucket).serialize16c},
	}

	timestamps := slices.Sorted(maps.Keys(a.buckets))
	fmt.Printf("Available timestamps: %#v\n", timestamps)
	if *flagTs == 0 {
		*flagTs = timestamps[0]
	}

	pids := collect(maps.All(a.buckets[*flagTs]))
	stdsort.Slice(pids, func(i, j int) bool { return pids[i].b.length() > pids[j].b.length() })
	fmt.Printf("Available pids:\n")
	for _, pid := range pids {
		fmt.Printf("  % 8d: %d\n", pid.a, pid.b.length())
	}

	if *flagPid == 0 {
		*flagPid = pids[0].a
	}

	b := a.buckets[*flagTs][*flagPid]
	b.printStats()

	type result struct {
		size int
		time float64
	}

	results := make([][20]result, len(funcs))
	levels := []int{1, 5, 19}
	for item_no, item := range funcs {
		m, _ := fmt.Printf("\n%s\n", item.name)
		fmt.Printf("%s\n", strings.Repeat("-", m-2))

		t0 := time.Now()

		raw := bytes.NewBuffer([]byte{})
		item.fn(b, raw)

		t1 := time.Now()

		results[item_no][0] = result{
			size: raw.Len(),
			time: t1.Sub(t0).Seconds(),
		}

		fmt.Printf("raw = %d, t1 = %v\n", raw.Len(), t1.Sub(t0).Seconds())
		os.WriteFile(fmt.Sprintf("%s.pb", item.name), raw.Bytes(), 0666)

		compressed := bytes.NewBuffer(nil)

		for _, level := range levels {
			t1 := time.Now()
			compressed.Reset()
			w := zstd.NewWriterLevel(compressed, level)
			w.Write(raw.Bytes())
			w.Close()
			t2 := time.Now()
			results[item_no][level] = result{
				size: compressed.Len(),
				time: t2.Sub(t1).Seconds(),
			}
			os.WriteFile(fmt.Sprintf("%s.pb.ztd.%02d", item.name, level), compressed.Bytes(), 0666)
		}

	}

	if *flagProfMem != "" {
		f, err := os.Create(*flagProfMem)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		runtime.GC()
		if err := pprof.Lookup("allocs").WriteTo(f, 0); err != nil {
			panic(err)
		}
	}

	levels = append([]int{0}, levels...)
	for item_no, item := range funcs {
		fmt.Printf("%s\t", item.name)
		for _, level := range levels {
			r := results[item_no][level]
			fmt.Printf("\t%d\t%.3f", r.size, r.time)
		}
		fmt.Printf("\n")
	}
}

type pair[A, B any] struct {
	a A
	b B
}

func collect[A, B any](seq iter.Seq2[A, B]) []pair[A, B] {
	s := []pair[A, B]{}
	for a, b := range seq {
		s = append(s, pair[A, B]{a, b})
	}
	return s
}
