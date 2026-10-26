// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package golabels

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process/gooffsets"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/utils"
)

var errUnmapped = errors.New("unmapped")

// fakeMemory is a sparse address space, allocated bump-pointer style.
type fakeMemory struct {
	regions map[uint64][]byte
	next    uint64
}

func newFakeMemory() *fakeMemory {
	return &fakeMemory{regions: make(map[uint64][]byte), next: 0x10000}
}

// Read implements Memory.
func (m *fakeMemory) Read(addr uint64, buf []byte) error {
	for base, data := range m.regions {
		if addr >= base && addr+uint64(len(buf)) <= base+uint64(len(data)) {
			copy(buf, data[addr-base:])
			return nil
		}
	}
	return errUnmapped
}

// alloc maps size zeroed bytes and returns their address.
func (m *fakeMemory) alloc(size int) uint64 {
	addr := m.next
	m.regions[addr] = make([]byte, size)
	m.next += uint64(size+0xfff) &^ 0xfff
	return addr
}

func (m *fakeMemory) putWord(addr, value uint64) {
	for base, data := range m.regions {
		if addr >= base && addr+8 <= base+uint64(len(data)) {
			binary.NativeEndian.PutUint64(data[addr-base:], value)
			return
		}
	}
	panic("putWord out of any region")
}

// putString maps s and writes its string header at hdr.
func (m *fakeMemory) putString(hdr uint64, s string) {
	if s == "" {
		return
	}
	data := m.alloc(len(s))
	copy(m.regions[data], s)
	m.putWord(hdr, data)
	m.putWord(hdr+8, uint64(len(s)))
}

// The offsets of an amd64 go1.25 and go1.23, out of goruntime_offsets.go.
var (
	sliceOffsets = gooffsets.GoRuntimeOffsets{MOffset: 48, MGsignal: 72, Curg: 184, Labels: 344}
	mapOffsets   = gooffsets.GoRuntimeOffsets{MOffset: 48, MGsignal: 80, Curg: 192, Labels: 352, HmapCount: 0, HmapLog2BucketCount: 9, HmapBuckets: 16}
)

// goroutine maps a g, its m and the curg of that m, and returns g and curg.
func goroutine(mem *fakeMemory, offsets gooffsets.GoRuntimeOffsets) (g, curg uint64) {
	g = mem.alloc(512)
	m := mem.alloc(512)
	curg = mem.alloc(512)
	mem.putWord(g+uint64(offsets.MOffset), m)
	mem.putWord(m+uint64(offsets.Curg), curg)
	return g, curg
}

// sliceLabels sets labels on curg the way Go >= 1.24 stores them.
func sliceLabels(mem *fakeMemory, curg uint64, labels []Label) {
	array := mem.alloc(max(len(labels), 1) * 2 * goStringSize)
	for i, label := range labels {
		mem.putString(array+uint64(i)*2*goStringSize, label.Key)
		mem.putString(array+uint64(i)*2*goStringSize+goStringSize, label.Value)
	}
	slice := mem.alloc(goSliceSize)
	mem.putWord(slice, array)
	mem.putWord(slice+8, uint64(len(labels)))
	mem.putWord(slice+16, uint64(len(labels)))
	mem.putWord(curg+uint64(sliceOffsets.Labels), slice)
}

// mapLabels sets labels on curg the way Go < 1.24 stores them, all in the
// first bucket.
func mapLabels(mem *fakeMemory, curg uint64, labels []Label) {
	bucket := mem.alloc(goMapBucketBytes)
	for slot, label := range labels {
		mem.regions[bucket][slot] = 0x80 // any tophash but empty
		mem.putString(bucket+goMapBucketSize+uint64(slot)*goStringSize, label.Key)
		mem.putString(bucket+goMapBucketSize+uint64(goMapBucketSize+slot)*goStringSize, label.Value)
	}
	hmap := mem.alloc(64)
	mem.putWord(hmap+uint64(mapOffsets.HmapCount), uint64(len(labels)))
	mem.putWord(hmap+uint64(mapOffsets.HmapBuckets), bucket)
	labelMap := mem.alloc(8)
	mem.putWord(labelMap, hmap)
	mem.putWord(curg+uint64(mapOffsets.Labels), labelMap)
}

var ddLabels = []Label{
	{Key: "span id", Value: "1234"},
	{Key: "local root span id", Value: "5678"},
	{Key: "trace id", Value: "000000000000162e0000000000000004"},
}

func TestReadLabels(t *testing.T) {
	t.Run("slice_tls", func(t *testing.T) {
		mem := newFakeMemory()
		g, curg := goroutine(mem, sliceOffsets)
		sliceLabels(mem, curg, ddLabels)

		tls := mem.alloc(64)
		tp := tls + 32
		mem.putWord(tp-8, g)

		layout := Layout{offsets: sliceOffsets, tlsOffset: -8}
		labels, status := layout.ReadLabels(mem, tp, 0)
		require.Equal(t, ReadOK, status)
		assert.Equal(t, ddLabels, labels)

		spanID, traceID := SpanContext(labels)
		assert.Equal(t, uint64(1234), spanID)
		assert.Equal(t, utils.TraceID{Hi: 0x162e, Lo: 4}, traceID)
	})

	t.Run("slice_g_register", func(t *testing.T) {
		// a !iscgo arm64 binary: no TLS offset, g comes from R28
		mem := newFakeMemory()
		g, curg := goroutine(mem, sliceOffsets)
		sliceLabels(mem, curg, ddLabels)

		layout := Layout{offsets: sliceOffsets}
		labels, status := layout.ReadLabels(mem, 0, g)
		require.Equal(t, ReadOK, status)
		assert.Equal(t, ddLabels, labels)
	})

	t.Run("map", func(t *testing.T) {
		mem := newFakeMemory()
		g, curg := goroutine(mem, mapOffsets)
		mapLabels(mem, curg, ddLabels)

		layout := Layout{offsets: mapOffsets}
		labels, status := layout.ReadLabels(mem, 0, g)
		require.Equal(t, ReadOK, status)
		assert.ElementsMatch(t, ddLabels, labels)
	})

	t.Run("limits", func(t *testing.T) {
		mem := newFakeMemory()
		g, curg := goroutine(mem, sliceOffsets)
		var many []Label
		for range MaxPairs + 2 {
			many = append(many, Label{Key: strings.Repeat("k", 40), Value: strings.Repeat("v", 80)})
		}
		sliceLabels(mem, curg, many)

		layout := Layout{offsets: sliceOffsets}
		labels, status := layout.ReadLabels(mem, 0, g)
		require.Equal(t, ReadOK, status)
		require.Len(t, labels, MaxPairs)
		assert.Len(t, labels[0].Key, KeySize-1)
		assert.Len(t, labels[0].Value, ValSize-1)
	})

	t.Run("no_labels", func(t *testing.T) {
		mem := newFakeMemory()
		g, _ := goroutine(mem, sliceOffsets)

		layout := Layout{offsets: sliceOffsets}
		_, status := layout.ReadLabels(mem, 0, g)
		assert.Equal(t, ReadNone, status)
	})

	t.Run("no_curg", func(t *testing.T) {
		mem := newFakeMemory()
		g := mem.alloc(512)
		m := mem.alloc(512)
		mem.putWord(g+uint64(sliceOffsets.MOffset), m)

		layout := Layout{offsets: sliceOffsets}
		_, status := layout.ReadLabels(mem, 0, g)
		assert.Equal(t, ReadNone, status)
	})

	t.Run("no_thread_pointer", func(t *testing.T) {
		layout := Layout{offsets: sliceOffsets, tlsOffset: -8}
		_, status := layout.ReadLabels(newFakeMemory(), 0, 0)
		assert.Equal(t, ReadNoThreadPointer, status)
	})

	t.Run("g_not_found", func(t *testing.T) {
		mem := newFakeMemory()
		tp := mem.alloc(64) + 32

		layout := Layout{offsets: sliceOffsets, tlsOffset: -8}
		_, status := layout.ReadLabels(mem, tp, 0)
		assert.Equal(t, ReadGNotFound, status)
	})

	t.Run("fault", func(t *testing.T) {
		layout := Layout{offsets: sliceOffsets}
		_, status := layout.ReadLabels(newFakeMemory(), 0, 0xdead0000)
		assert.Equal(t, ReadFault, status)
	})
}

func TestSpanContext(t *testing.T) {
	t.Run("legacy_trace_id", func(t *testing.T) {
		spanID, traceID := SpanContext([]Label{
			{Key: "span id", Value: "42"},
			{Key: "local root span id", Value: "43"},
		})
		assert.Equal(t, uint64(42), spanID)
		assert.Equal(t, utils.TraceID{Lo: 43}, traceID)
	})

	t.Run("none", func(t *testing.T) {
		spanID, traceID := SpanContext([]Label{{Key: "other", Value: "1"}})
		assert.Zero(t, spanID)
		assert.Equal(t, utils.TraceID{}, traceID)
	})
}
