// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package metrics

import (
	"fmt"
	"math"

	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/DataDog/sketches-go/ddsketch/mapping"
	"github.com/DataDog/sketches-go/ddsketch/store"
)

// expoBucketConverter fills DDSketch stores from the buckets of an exponential
// histogram, rebasing each boundary into nanoseconds on the way.
//
// It also collects the observations that belong in the sketch's zero bin, so the
// two halves of a data point have to be converted through the same instance, and
// an instance must not be reused across data points.
type expoBucketConverter struct {
	// mapping turns a boundary in nanoseconds into a store index.
	mapping *mapping.LogarithmicMapping
	// base is the ratio between two consecutive boundaries of the input
	// histogram, and unitScale converts a boundary to nanoseconds.
	base      float64
	unitScale float64
	// minValue and maxValue delimit what mapping can index; degenerate records
	// that the two leave no value at all. A LogarithmicMapping computes its
	// indexable range once, at construction time.
	minValue   float64
	maxValue   float64
	degenerate bool
	// zeroBinCount accumulates the observations that belong in the sketch's zero
	// bin, across both halves of the data point.
	zeroBinCount float64
}

func newExpoBucketConverter(m *mapping.LogarithmicMapping, base, unitScale float64) *expoBucketConverter {
	minValue, maxValue := m.MinIndexableValue(), m.MaxIndexableValue()
	return &expoBucketConverter{
		mapping:   m,
		base:      base,
		unitScale: unitScale,
		minValue:  minValue,
		maxValue:  maxValue,
		// Negated rather than written as minValue > maxValue so that a range
		// carrying a NaN bound, which no value can fall inside either, counts as
		// degenerate too.
		degenerate: !(minValue <= maxValue),
	}
}

// boundary returns the lower bound of the given bucket, in nanoseconds.
func (c *expoBucketConverter) boundary(bucketIndex int) float64 {
	return math.Pow(c.base, float64(bucketIndex)) * c.unitScale
}

// convert turns one half of an exponential histogram into a store.
//
// LogarithmicMapping.Index does not validate its argument. It computes log(value) * multiplier and
// converts the result to an int. Consequently, a boundary outside the  mapping’s  indexable  range
// produces an index that the dense store cannot use as a slice offset, causing the store to panic.
// Since the base is greater than one, boundary values increase with their bucket indices, so it is
// sufficient to clear the largest boundary before indexing any of them.
func (c *expoBucketConverter) convert(b pmetric.ExponentialHistogramDataPointBuckets) (store.Store, error) {
	offset := int(b.Offset())
	bucketCounts := b.BucketCounts()
	valueStore := store.NewDenseStore()

	// Empty buckets hold no observation and the dense store would drop them
	// anyway, so the largest boundary that gets asked for belongs to the last
	// populated bucket.
	last := -1
	for j := bucketCounts.Len() - 1; j >= 0; j-- {
		if bucketCounts.At(j) > 0 {
			last = j
			break
		}
	}
	if last < 0 {
		// Nothing here needs the mapping, so nothing about it can be wrong.
		return valueStore, nil
	}

	// A scale fine enough to make gamma barely greater than one gives the mapping
	// a multiplier so large that its indexable range inverts, leaving no value it
	// can represent. Reject instead of folding every observation into the zero bin.
	if c.degenerate {
		return nil, fmt.Errorf("mapping has no indexable value: its indexable range is [%g, %g] ns", c.minValue, c.maxValue)
	}
	// Negated so that a NaN top, which no comparison would catch, is rejected too.
	if top := c.boundary(last + offset); !(top <= c.maxValue) {
		return nil, fmt.Errorf("bucket %d has a boundary of %g ns, above the largest the mapping can index (%g ns)", last+offset, top, c.maxValue)
	}

	// Every remaining boundary is at most that one, so none can exceed the
	// mapping either. What is left is the prefix too small for it to tell apart
	// from zero.
	for j := 0; j <= last; j++ {
		count := bucketCounts.At(j)
		if count == 0 {
			continue
		}
		if boundary := c.boundary(j + offset); boundary < c.minValue {
			// DDSketch keeps such observations in its zero bin, so they survive.
			c.zeroBinCount += float64(count)
		} else {
			// Keep the input's precision by reusing the same gamma.
			valueStore.AddWithCount(c.mapping.Index(boundary), float64(count))
		}
	}
	return valueStore, nil
}
