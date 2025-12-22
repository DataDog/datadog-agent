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
	"github.com/DataDog/sketches-go/ddsketch/store"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

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
