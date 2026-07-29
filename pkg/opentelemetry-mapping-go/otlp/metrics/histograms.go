// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package metrics

import (
	"errors"
	"fmt"
	"math"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/DataDog/sketches-go/ddsketch/mapping"
	"github.com/DataDog/sketches-go/ddsketch/store"

	"github.com/DataDog/datadog-agent/pkg/util/quantile"
)

// These methods will be imported in dd-go to convert OTLP to DD trace metrics - see https://github.com/DataDog/dd-go/pull/198508

// getTimeUnitScaleToNanos returns the scaling factor to convert the given unit to nanoseconds
func getTimeUnitScaleToNanos(unit string) float64 {
	switch unit {
	case "ns":
		return float64(time.Nanosecond)
	case "us", "μs":
		return float64(time.Microsecond)
	case "ms":
		return float64(time.Millisecond)
	case "s":
		return float64(time.Second)
	case "min":
		return float64(time.Minute)
	case "h":
		return float64(time.Hour)
	default:
		// If unit is unknown, assume seconds (common for duration metrics)
		return float64(time.Second)
	}
}

// getBounds returns the lower and upper bounds for a histogram bucket
func getBounds(explicitBounds pcommon.Float64Slice, idx int) (lowerBound float64, upperBound float64) {
	// See https://github.com/open-telemetry/opentelemetry-proto/blob/v0.10.0/opentelemetry/proto/metrics/v1/metrics.proto#L427-L439
	lowerBound = math.Inf(-1)
	upperBound = math.Inf(1)
	if idx > 0 {
		lowerBound = explicitBounds.At(idx - 1)
	}
	if idx < explicitBounds.Len() {
		upperBound = explicitBounds.At(idx)
	}
	return
}

// CreateDDSketchFromHistogramOfDuration creates a DDSketch from regular histogram data point
func CreateDDSketchFromHistogramOfDuration(dp *pmetric.HistogramDataPoint, unit string) (*ddsketch.DDSketch, error) {
	relativeAccuracy := 0.01 // 1% relative accuracy
	maxNumBins := 2048
	newSketch, err := ddsketch.LogCollapsingLowestDenseDDSketch(relativeAccuracy, maxNumBins)
	if err != nil {
		return nil, err
	}

	if dp == nil {
		return newSketch, nil
	}

	bucketCounts := dp.BucketCounts()
	explicitBounds := dp.ExplicitBounds()

	// A histogram without buckets conveys its population via only sum and count (OTLP
	// spec) and is interpreted as a single bucket covering (-Inf, +Inf); synthesize it.
	if bucketCounts.Len() == 0 {
		bucketCounts = pcommon.NewUInt64Slice()
		explicitBounds = pcommon.NewFloat64Slice()
		if dp.HasMin() {
			bucketCounts.Append(0)
			explicitBounds.Append(dp.Min())
		}
		bucketCounts.Append(dp.Count())
		if dp.HasMax() {
			bucketCounts.Append(0)
			explicitBounds.Append(dp.Max())
		}
	}

	if bucketCounts.Len() != explicitBounds.Len()+1 {
		return nil, errors.New("bucket counts length does not match explicit bounds length")
	}

	// Get scaling factor to convert unit to nanoseconds
	scaleToNanos := getTimeUnitScaleToNanos(unit)

	// Find first and last bucket indices with count > 0
	lowestBucketIndex := -1
	highestBucketIndex := -1
	for j := 0; j < bucketCounts.Len(); j++ {
		count := bucketCounts.At(j)
		if count > 0 {
			if lowestBucketIndex == -1 {
				lowestBucketIndex = j
			}
			highestBucketIndex = j
		}
	}

	hasMin := dp.HasMin()
	hasMax := dp.HasMax()
	minNanoseconds := dp.Min() * scaleToNanos
	maxNanoseconds := dp.Max() * scaleToNanos

	for j := 0; j < bucketCounts.Len(); j++ {
		lowerBound, upperBound := getBounds(explicitBounds, j)

		if math.IsInf(upperBound, 1) {
			upperBound = lowerBound
		} else if math.IsInf(lowerBound, -1) {
			lowerBound = upperBound
		}

		count := bucketCounts.At(j)

		if count > 0 {
			insertionPoint := 0.0
			adjustedCount := float64(count)
			midpoint := (lowerBound + upperBound) / 2 * scaleToNanos
			// Determine insertion point based on bucket position
			if j == lowestBucketIndex && j == highestBucketIndex {
				// Special case: min and max are in the same bucket
				if hasMin && hasMax {
					insertionPoint = (minNanoseconds + maxNanoseconds) / 2
				}
			} else if j == lowestBucketIndex {
				// Bottom bucket: insert at min value
				if hasMin {
					insertionPoint = minNanoseconds
				}
			} else if j == highestBucketIndex {
				// Top bucket: insert at max value
				if hasMax {
					insertionPoint = maxNanoseconds
				}
			}

			if insertionPoint == 0.0 {
				insertionPoint = midpoint
			}

			err := newSketch.AddWithCount(insertionPoint, adjustedCount)
			if err != nil {
				return nil, fmt.Errorf("failed to add value to DDSketch: %w", err)
			}
		}
	}

	return newSketch, nil
}

func toStoreFromExponentialBucketsWithUnitScale(b pmetric.ExponentialHistogramDataPointBuckets, mapping *mapping.LogarithmicMapping, base float64, scaleToNanos float64) store.Store {
	offset := b.Offset()
	bucketCounts := b.BucketCounts()

	store := store.NewDenseStore()
	for j := 0; j < bucketCounts.Len(); j++ {
		bucketIndex := j + int(offset)
		count := bucketCounts.At(j)

		if count > 0 {
			// Calculate the actual bucket boundary value
			bucketValue := math.Pow(base, float64(bucketIndex))

			// Scale the bucket value to nanoseconds
			scaledValue := bucketValue * scaleToNanos

			// Convert back to the index in the nanosecond space
			// Using the same gamma since we're keeping the same precision
			scaledIndex := mapping.Index(scaledValue)
			store.AddWithCount(scaledIndex, float64(count))
		}
	}
	return store
}

// CreateDDSketchFromExponentialHistogramOfDuration creates a DDSketch from exponential histogram data point
func CreateDDSketchFromExponentialHistogramOfDuration(p *pmetric.ExponentialHistogramDataPoint, scale int32, unit string) (*ddsketch.DDSketch, error) {
	// Create the DDSketch stores
	scaleToNanos := getTimeUnitScaleToNanos(unit)

	// Create the DDSketch mapping that corresponds to the ExponentialHistogram settings
	gammaWithOnePercentAccuracy := 1.01 / 0.99
	gamma := math.Pow(2, math.Pow(2, float64(-scale)))
	gamma = math.Min(gamma, gammaWithOnePercentAccuracy)
	indexOffset := math.Log(scaleToNanos)
	mapping, err := mapping.NewLogarithmicMappingWithGamma(gamma, indexOffset)
	if err != nil {
		return nil, fmt.Errorf("couldn't create LogarithmicMapping for DDSketch: %w", err)
	}

	// Calculate the base for the exponential histogram
	base := math.Pow(2, math.Pow(2, float64(-scale)))
	var positiveStore store.Store
	var negativeStore store.Store
	var zeroCount float64
	if p != nil {
		positiveStore = toStoreFromExponentialBucketsWithUnitScale(p.Positive(), mapping, base, scaleToNanos)
		negativeStore = toStoreFromExponentialBucketsWithUnitScale(p.Negative(), mapping, base, scaleToNanos)
		zeroCount = float64(p.ZeroCount())
	} else {
		positiveStore = store.NewDenseStore()
		negativeStore = store.NewDenseStore()
	}

	// Create DDSketch with the above mapping and stores
	sketch := ddsketch.NewDDSketch(mapping, positiveStore, negativeStore)
	err = sketch.AddWithCount(0, zeroCount)
	if err != nil {
		return nil, fmt.Errorf("failed to add ZeroCount to DDSketch: %w", err)
	}

	return sketch, nil
}

// SketchExactSummary carries exact aggregate values reported by an OTLP histogram, used
// to correct a converted Sketch whose bucket-midpoint reconstruction only approximates them.
type SketchExactSummary struct {
	Count    uint64
	HasCount bool
	Sum      float64
	HasSum   bool
	Min      float64
	HasMin   bool
	Max      float64
	HasMax   bool
}

// OverwriteSketchSummary overwrites agentSketch.Basic with whichever fields exact reports
// as present. Avg is recomputed from Sum/Cnt whenever Sum is overwritten.
func OverwriteSketchSummary(agentSketch *quantile.Sketch, exact SketchExactSummary) {
	if exact.HasCount {
		agentSketch.Basic.Cnt = int64(exact.Count)
	}
	if exact.HasSum {
		agentSketch.Basic.Sum = exact.Sum
		if agentSketch.Basic.Cnt > 0 {
			agentSketch.Basic.Avg = agentSketch.Basic.Sum / float64(agentSketch.Basic.Cnt)
		}
	}
	if exact.HasMin {
		agentSketch.Basic.Min = exact.Min
	}
	if exact.HasMax {
		agentSketch.Basic.Max = exact.Max
	}
}
