// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metrics

import (
	"fmt"
	"math"

	"github.com/DataDog/sketches-go/ddsketch/store"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

// maxFiniteLog2 is the largest base-2 exponent of a finite float64: 2^1024 overflows to +Inf.
const maxFiniteLog2 = 1023

func toStore(b pmetric.ExponentialHistogramDataPointBuckets) store.Store {
	offset := b.Offset()
	bucketCounts := b.BucketCounts()

	store := store.NewDenseStore()
	for j := 0; j < bucketCounts.Len(); j++ {
		// Find the real index of the bucket by adding the offset
		index := j + int(offset)

		store.AddWithCount(index, float64(bucketCounts.At(j)))
	}
	return store
}

// maxPopulatedIndex returns the highest bucket index with a non-zero count, and
// whether any such bucket exists. Zero-count buckets are excluded because
// DenseStore.AddWithCount discards them, so they never reach the mapping.
func maxPopulatedIndex(b pmetric.ExponentialHistogramDataPointBuckets) (int, bool) {
	offset := int(b.Offset())
	bucketCounts := b.BucketCounts()
	for j := bucketCounts.Len() - 1; j >= 0; j-- {
		if bucketCounts.At(j) > 0 {
			return j + offset, true
		}
	}
	return 0, false
}

// checkExponentialHistogramBounds reports whether the populated bucket boundaries
// of p can be represented as finite float64 values at the given scale.
//
// The boundary of bucket index i is gamma^i, with gamma = 2^(2^-scale), so its
// base-2 logarithm is i * 2^-scale. boundaryScaleFactor is an additional factor
// applied to every boundary by the caller (1 when there is none); its logarithm is
// added to the bound because it can overflow a boundary that is representable on
// its own.
//
// Boundaries that overflow to +Inf, and a gamma that itself overflows (scale <= -10,
// which makes the mapping's multiplier zero and the boundary of index 0 a NaN), both
// end up feeding a non-finite value to LogarithmicMapping.Index — either directly in
// toStoreFromExponentialBucketsWithUnitScale, or via DDSketch.ChangeMapping when the
// sketch is converted to an Agent sketch. Converting +Inf or NaN to an int is
// architecture dependent: on amd64 NaN and +Inf both yield math.MinInt64, on arm64
// the conversion saturates. Either way DenseStore uses the result as a slice offset
// and panics. Reject such points here instead.
//
// Underflow on the negative side is not rejected: boundaries that round down to
// zero leave ChangeMapping's remapping loop empty rather than panicking.
func checkExponentialHistogramBounds(p pmetric.ExponentialHistogramDataPoint, scale int32, boundaryScaleFactor float64) error {
	// gamma = 2^(2^-scale) overflows for any scale <= -10. The mapping is then
	// unusable for every index, including the negative ones, so reject outright.
	if gamma := math.Pow(2, math.Pow(2, float64(-scale))); math.IsInf(gamma, 0) {
		return fmt.Errorf("exponential histogram scale %d is too small: gamma overflows float64", scale)
	}

	log2ScaleFactor := math.Log2(boundaryScaleFactor)

	for _, buckets := range []pmetric.ExponentialHistogramDataPointBuckets{p.Positive(), p.Negative()} {
		maxIndex, ok := maxPopulatedIndex(buckets)
		if !ok {
			continue
		}
		// The remapping reads the boundary of maxIndex+1 as the bucket's upper bound.
		// math.Ldexp computes (maxIndex+1) * 2^-scale without an intermediate overflow.
		log2Bound := math.Ldexp(float64(maxIndex+1), int(-scale)) + log2ScaleFactor
		if log2Bound > maxFiniteLog2 {
			return fmt.Errorf(
				"exponential histogram bucket index %d is out of range for scale %d: boundary 2^%g overflows float64",
				maxIndex, scale, log2Bound,
			)
		}
	}
	return nil
}
