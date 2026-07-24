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

	"github.com/DataDog/sketches-go/ddsketch/mapping"
	"github.com/DataDog/sketches-go/ddsketch/store"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func toStore(
	b pmetric.ExponentialHistogramDataPointBuckets,
	indexMapping *mapping.LogarithmicMapping,
) (store.Store, error) {
	offset := b.Offset()
	bucketCounts := b.BucketCounts()
	bucketCount := bucketCounts.Len()

	denseStore := store.NewDenseStore()
	firstNonEmpty := 0
	for firstNonEmpty < bucketCount && bucketCounts.At(firstNonEmpty) == 0 {
		firstNonEmpty++
	}
	if firstNonEmpty == bucketCount {
		return denseStore, nil
	}

	lastNonEmpty := bucketCount - 1
	for bucketCounts.At(lastNonEmpty) == 0 {
		lastNonEmpty--
	}

	// LowerBound is monotonic, so checking the occupied range endpoints avoids
	// computing two exponential bounds for every non-empty bucket.
	minIndex := firstNonEmpty + int(offset)
	maxIndex := lastNonEmpty + int(offset) + 1
	lowerBound := indexMapping.LowerBound(minIndex)
	upperBound := indexMapping.LowerBound(maxIndex)
	if math.IsNaN(lowerBound) ||
		math.IsNaN(upperBound) ||
		math.IsInf(lowerBound, 0) ||
		math.IsInf(upperBound, 0) ||
		lowerBound <= 0 ||
		upperBound <= lowerBound {
		return nil, fmt.Errorf("bucket index range [%d, %d) has unsupported bounds [%v, %v)", minIndex, maxIndex, lowerBound, upperBound)
	}

	for j := firstNonEmpty; j <= lastNonEmpty; j++ {
		count := bucketCounts.At(j)
		if count != 0 {
			denseStore.AddWithCount(j+int(offset), float64(count))
		}
	}
	return denseStore, nil
}
