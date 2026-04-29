// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/binary"
	"hash"
	"hash/fnv"
	"math"
	"sort"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// readDigest captures the cumulative hash of all storage reads during a single
// Detect() call, along with metadata for matching across live/replay runs.
type readDigest struct {
	DetectorName string `json:"detector"`
	DataTime     int64  `json:"data_time"`
	Hash         uint64 `json:"hash"`
	ReadCount    int    `json:"read_count"`
	PointCount   int    `json:"point_count"`
}

// instrumentedStorage wraps a StorageReader and hashes every read to produce
// a deterministic digest of what a detector actually saw.
//
// Each storage call produces an independent hash. At digest time, these
// per-call hashes are sorted and combined so the final hash is independent
// of the order detectors call storage methods. This is critical because
// ListSeries may return series in different order between live and replay.
type instrumentedStorage struct {
	inner observerdef.StorageReader

	callHashes []uint64 // one hash per storage call
	readCount  int
	pointCount int
}

func newInstrumentedStorage(inner observerdef.StorageReader) *instrumentedStorage {
	return &instrumentedStorage{inner: inner}
}

func (s *instrumentedStorage) reset() {
	s.callHashes = s.callHashes[:0]
	s.readCount = 0
	s.pointCount = 0
}

func (s *instrumentedStorage) digest(detectorName string, dataTime int64) readDigest {
	// Combine per-call hashes in sorted order for order-independence.
	sorted := make([]uint64, len(s.callHashes))
	copy(sorted, s.callHashes)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	h := fnv.New64a()
	var buf [8]byte
	for _, ch := range sorted {
		binary.LittleEndian.PutUint64(buf[:], ch)
		h.Write(buf[:])
	}

	return readDigest{
		DetectorName: detectorName,
		DataTime:     dataTime,
		Hash:         h.Sum64(),
		ReadCount:    s.readCount,
		PointCount:   s.pointCount,
	}
}

// callHasher builds a hash for a single storage call.
type callHasher struct {
	h hash.Hash64
}

func newCallHasher() *callHasher {
	return &callHasher{h: fnv.New64a()}
}

func (c *callHasher) mixString(v string) { c.h.Write([]byte(v)) }
func (c *callHasher) mixUint64(v uint64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], v)
	c.h.Write(b[:])
}
func (c *callHasher) mixInt64(v int64)     { c.mixUint64(uint64(v)) }
func (c *callHasher) mixFloat64(v float64) { c.mixUint64(math.Float64bits(v)) }
func (c *callHasher) sum() uint64          { return c.h.Sum64() }

func (c *callHasher) mixSeriesIdentity(namespace, name string, tags []string) {
	c.mixString(namespace)
	c.mixString(name)
	c.mixInt64(int64(len(tags)))
	for _, tag := range tags {
		c.mixString(tag)
	}
}

func (c *callHasher) mixSeries(series *observerdef.Series) int {
	if series == nil {
		c.mixUint64(0)
		return 0
	}
	c.mixUint64(1)
	c.mixSeriesIdentity(series.Namespace, series.Name, series.Tags)
	c.mixInt64(int64(len(series.Points)))
	for _, p := range series.Points {
		c.mixInt64(p.Timestamp)
		c.mixFloat64(p.Value)
	}
	return len(series.Points)
}

// --- StorageReader interface ---

func (s *instrumentedStorage) ListSeries(filter observerdef.SeriesFilter) []observerdef.SeriesMeta {
	s.readCount++
	result := s.inner.ListSeries(filter)

	// Hash each series identity independently, then sort and combine.
	// ListSeries iterates a Go map internally, so slice order is non-deterministic.
	perSeries := make([]uint64, len(result))
	for i, m := range result {
		ih := newCallHasher()
		ih.mixSeriesIdentity(m.Namespace, m.Name, m.Tags)
		perSeries[i] = ih.sum()
	}
	sort.Slice(perSeries, func(i, j int) bool { return perSeries[i] < perSeries[j] })

	ch := newCallHasher()
	ch.mixString("ListSeries")
	ch.mixString(filter.Namespace)
	ch.mixInt64(int64(len(result)))
	for _, h := range perSeries {
		ch.mixUint64(h)
	}
	s.callHashes = append(s.callHashes, ch.sum())
	return result
}

func (s *instrumentedStorage) GetSeriesRange(ref observerdef.SeriesRef, start, end int64, agg observerdef.Aggregate) *observerdef.Series {
	s.readCount++
	result := s.inner.GetSeriesRange(ref, start, end, agg)

	ch := newCallHasher()
	ch.mixString("GetSeriesRange")
	ch.mixInt64(start)
	ch.mixInt64(end)
	ch.mixInt64(int64(agg))
	pts := ch.mixSeries(result)
	s.pointCount += pts
	s.callHashes = append(s.callHashes, ch.sum())
	return result
}

func (s *instrumentedStorage) ForEachPoint(ref observerdef.SeriesRef, start, end int64, agg observerdef.Aggregate, fn func(*observerdef.Series, observerdef.Point)) bool {
	s.readCount++

	ch := newCallHasher()
	ch.mixString("ForEachPoint")
	ch.mixInt64(start)
	ch.mixInt64(end)
	ch.mixInt64(int64(agg))

	var callPointCount int
	var identityHashed bool
	found := s.inner.ForEachPoint(ref, start, end, agg, func(series *observerdef.Series, p observerdef.Point) {
		if !identityHashed {
			ch.mixSeriesIdentity(series.Namespace, series.Name, series.Tags)
			identityHashed = true
		}
		ch.mixInt64(p.Timestamp)
		ch.mixFloat64(p.Value)
		callPointCount++
		s.pointCount++
		fn(series, p)
	})

	ch.mixInt64(int64(callPointCount))
	if found {
		ch.mixUint64(1)
	} else {
		ch.mixUint64(0)
	}
	s.callHashes = append(s.callHashes, ch.sum())
	return found
}

func (s *instrumentedStorage) PointCount(ref observerdef.SeriesRef) int {
	s.readCount++
	result := s.inner.PointCount(ref)

	ch := newCallHasher()
	ch.mixString("PointCount")
	ch.mixInt64(int64(result))
	s.callHashes = append(s.callHashes, ch.sum())
	return result
}

func (s *instrumentedStorage) PointCountUpTo(ref observerdef.SeriesRef, endTime int64) int {
	s.readCount++
	result := s.inner.PointCountUpTo(ref, endTime)

	ch := newCallHasher()
	ch.mixString("PointCountUpTo")
	ch.mixInt64(endTime)
	ch.mixInt64(int64(result))
	s.callHashes = append(s.callHashes, ch.sum())
	return result
}

func (s *instrumentedStorage) SumRange(ref observerdef.SeriesRef, start, end int64, agg observerdef.Aggregate) float64 {
	s.readCount++
	result := s.inner.SumRange(ref, start, end, agg)

	ch := newCallHasher()
	ch.mixString("SumRange")
	ch.mixInt64(int64(ref))
	ch.mixInt64(start)
	ch.mixInt64(end)
	ch.mixInt64(int64(agg))
	ch.mixFloat64(result)
	s.callHashes = append(s.callHashes, ch.sum())
	return result
}

func (s *instrumentedStorage) WriteGeneration(ref observerdef.SeriesRef) int64 {
	s.readCount++
	result := s.inner.WriteGeneration(ref)

	ch := newCallHasher()
	ch.mixString("WriteGeneration")
	ch.mixInt64(result)
	s.callHashes = append(s.callHashes, ch.sum())
	return result
}

func (s *instrumentedStorage) SeriesGeneration() uint64 {
	s.readCount++
	result := s.inner.SeriesGeneration()

	ch := newCallHasher()
	ch.mixString("SeriesGeneration")
	ch.mixUint64(result)
	s.callHashes = append(s.callHashes, ch.sum())
	return result
}
