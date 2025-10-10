// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package metrics

import (
	"fmt"
	"math"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/DataDog/sketches-go/ddsketch/mapping"
	"github.com/DataDog/sketches-go/ddsketch/store"
)

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
func CreateDDSketchFromHistogramOfDuration(dp pmetric.HistogramDataPoint, unit string) (*ddsketch.DDSketch, error) {
	relativeAccuracy := 0.01 // 1% relative accuracy
	maxNumBins := 2048
	newSketch, err := ddsketch.LogCollapsingLowestDenseDDSketch(relativeAccuracy, maxNumBins)
	if err != nil {
		return nil, err
	}

	bucketCounts := dp.BucketCounts()
	explicitBounds := dp.ExplicitBounds()

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

			newSketch.AddWithCount(insertionPoint, adjustedCount)
		}
	}

	return newSketch, nil
}

// CreateDDSketchFromExponentialHistogramOfDuration creates a DDSketch from exponential histogram data point
func _CreateDDSketchFromExponentialHistogramOfDuration(dp pmetric.ExponentialHistogramDataPoint, unit string) (*ddsketch.DDSketch, error) {
	// Get scaling factor to convert unit to nanoseconds
	scaleToNanos := getTimeUnitScaleToNanos(unit)

	// Create the DDSketch mapping for nanoseconds (no offset needed since we scaled the stores)
	gamma := math.Pow(2, math.Pow(2, float64(-dp.Scale())))
	offset := math.Log(scaleToNanos)
	mapping, err := mapping.NewLogarithmicMappingWithGamma(gamma, offset)
	if err != nil {
		return nil, fmt.Errorf("couldn't create LogarithmicMapping for DDSketch: %w", err)
	}

	// Create the DDSketch stores with unit scaling
	positiveStore := toStoreFromExponentialBucketsWithUnitScale(dp.Positive(), mapping, dp.Scale(), scaleToNanos)
	negativeStore := toStoreFromExponentialBucketsWithUnitScale(dp.Negative(), mapping, dp.Scale(), scaleToNanos)

	// Create DDSketch with the above mapping and stores
	sketch := ddsketch.NewDDSketch(mapping, positiveStore, negativeStore)

	// Zero count represents values at exactly zero, so we add at zero (no scaling needed for zero)
	err = sketch.AddWithCount(0, float64(dp.ZeroCount()))
	if err != nil {
		return nil, fmt.Errorf("failed to add ZeroCount to DDSketch: %w", err)
	}

	return sketch, nil
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
			// scaledIndex := int(math.Log(scaledValue) / math.Log(base))

			scaledIndex := mapping.Index(scaledValue)
			store.AddWithCount(scaledIndex, float64(count))
		}
	}
	return store
}

// func toStore2(b pmetric.ExponentialHistogramDataPointBuckets, scale float64) store.Store {
// 	offset := b.Offset()
// 	bucketCounts := b.BucketCounts()

// 	store := store.NewDenseStore()
// 	for j := 0; j < bucketCounts.Len(); j++ {
// 		// Find the real index of the bucket by adding the offset
// 		index := j + int(offset)

// 		store.AddWithCount(index, float64(bucketCounts.At(j)))
// 	}
// 	return store
// }

func CreateDDSketchFromExponentialHistogramOfDuration(p pmetric.ExponentialHistogramDataPoint, unit string) (*ddsketch.DDSketch, error) {
	// Create the DDSketch stores
	scaleToNanos := getTimeUnitScaleToNanos(unit)

	// Create the DDSketch mapping that corresponds to the ExponentialHistogram settings
	gamma := math.Pow(2, math.Pow(2, float64(-p.Scale())))
	gammaWithOnePercentAccuracy := 1.01 / 0.99
	gamma = math.Min(gamma, gammaWithOnePercentAccuracy)
	indexOffset := math.Log(scaleToNanos)
	mapping, err := mapping.NewLogarithmicMappingWithGamma(gamma, indexOffset)
	if err != nil {
		return nil, fmt.Errorf("couldn't create LogarithmicMapping for DDSketch: %w", err)
	}

	// Calculate the base for the exponential histogram
	base := math.Pow(2, math.Pow(2, float64(-p.Scale())))
	positiveStore := toStoreFromExponentialBucketsWithUnitScale(p.Positive(), mapping, base, scaleToNanos)
	negativeStore := toStoreFromExponentialBucketsWithUnitScale(p.Negative(), mapping, base, scaleToNanos)

	// Create DDSketch with the above mapping and stores
	sketch := ddsketch.NewDDSketch(mapping, positiveStore, negativeStore)
	err = sketch.AddWithCount(0, float64(p.ZeroCount()))
	if err != nil {
		return nil, fmt.Errorf("failed to add ZeroCount to DDSketch: %w", err)
	}

	return sketch, nil
}
