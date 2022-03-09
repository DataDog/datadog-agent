// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package quantile

import (
	"fmt"
	"math"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/quantile/summary"
	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/DataDog/sketches-go/ddsketch/mapping"
	"github.com/DataDog/sketches-go/ddsketch/store"
)

const (
	maxIndex = math.MaxInt16
)

// convertToCompatibleDDSketch converts any DDSketch into a DDSketch with parameters
// compatible with Sketch
func convertToCompatibleDDSketch(c *Config, inputSketch *ddsketch.DDSketch) (*ddsketch.DDSketch, error) {
	// Create positive store for the new DDSketch
	positiveStore := store.NewDenseStore()

	// Create negative store for the new DDSketch
	negativeStore := store.NewDenseStore()

	gamma := c.gamma.v
	offset := float64(c.norm.bias) + 0.5
	newMapping, err := mapping.NewLogarithmicMappingWithGamma(gamma, offset)
	if err != nil {
		return nil, fmt.Errorf("couldn't create LogarithmicMapping for DDSketch: %w", err)
	}

	newSketch := inputSketch.ChangeMapping(newMapping, positiveStore, negativeStore, 1.0)
	return newSketch, nil
}

type floatKeyCount struct {
	k int
	c float64
}

// keyCountsFromFloatKeyCounts converts DDSketch float counts to integer counts,
// preserving the total count of the sketch by tracking leftover decimal counts.
// TODO: this tends to shift the sketch towards the right (since the leftover counts
// get added to the rightmost bucket). This could be improved by adding leftover
// counts to the key that's the weighted average of keys that contribute to that leftover count.
func keyCountsFromFloatKeyCounts(floatKeyCounts []floatKeyCount) []KeyCount {
	keyCounts := make([]KeyCount, 0, len(floatKeyCounts))

	sort.Slice(floatKeyCounts, func(i, j int) bool {
		return floatKeyCounts[i].k < floatKeyCounts[j].k
	})

	leftoverCount := 0.0
	for _, fkc := range floatKeyCounts {
		key := fkc.k
		count := fkc.c

		// Add leftovers from previous bucket, and compute leftovers
		// for the next bucket
		count += leftoverCount
		uintCount := uint(count)
		leftoverCount = count - float64(uintCount)

		keyCounts = append(keyCounts, KeyCount{k: Key(key), n: uintCount})
	}

	// Edge case where there may be some leftover count because the total count
	// isn't an int (or due to float64 precision errors). In this case, round to
	// nearest.
	if leftoverCount >= 0.5 {
		lastIndex := len(keyCounts) - 1
		keyCounts[lastIndex] = KeyCount{k: keyCounts[lastIndex].k, n: keyCounts[lastIndex].n + 1}
	}

	return keyCounts
}

func fromCompatibleDDSketch(c *Config, inputSketch *ddsketch.DDSketch) (*Sketch, error) {
	sparseStore := sparseStore{
		bins:  make([]bin, 0, defaultBinListSize),
		count: 0,
	}

	// Special counter to aggregate all zeroes
	zeroes := 0.0

	floatKeyCounts := make([]floatKeyCount, 0, defaultBinListSize)

	signedStores := []struct {
		store store.Store
		sign  int
	}{
		{
			store: inputSketch.GetPositiveValueStore(),
			sign:  1,
		},
		{
			store: inputSketch.GetNegativeValueStore(),
			sign:  -1,
		},
	}

	for _, signedStore := range signedStores {
		var loopErr error

		signedStore.store.ForEach(func(index int, count float64) bool {
			if count <= 0 {
				loopErr = fmt.Errorf("negative counts are not supported: got %f", count)
				return true
			}

			if index >= maxIndex {
				loopErr = fmt.Errorf("index value %d exceeds the maximum supported index value (%d)", index, maxIndex)
				return true
			}

			// All indexes <= 0 represent values that are <= minValue,
			// and therefore end up in the zero bin
			if index <= 0 {
				// TODO: What if zeroes overflows float64? We may have
				// to keep multiple zeroes counters, and add one floatKeyCount for each.
				zeroes += count
				return false
			}

			// In the resulting Sketch, negative values are mapped to negative indexes, while
			// positive values are mapped to positive indexes, therefore we need to multiply
			// the index by the "sign" of the store.
			floatKeyCounts = append(floatKeyCounts, floatKeyCount{k: signedStore.sign * index, c: count})
			return false
		})

		if loopErr != nil {
			return nil, loopErr
		}
	}

	// The zero count of the sketch isn't directly exposed, compute it
	// TODO: Should DDSketch expose its zero count?
	zeroes += inputSketch.GetCount() - inputSketch.GetPositiveValueStore().TotalCount() - inputSketch.GetNegativeValueStore().TotalCount()

	// Finally, add the 0 key
	floatKeyCounts = append(floatKeyCounts, floatKeyCount{k: 0, c: zeroes})

	// Generate the integer KeyCount objects from the counts we retrieved
	keyCounts := keyCountsFromFloatKeyCounts(floatKeyCounts)

	// Populate sparseStore object with the collected keyCounts
	// insertCounts will take care of creating multiple uint16 bins for a
	// single key if the count overflows uint16
	sparseStore.insertCounts(c, keyCounts)

	// Create summary object
	// Calculate the total count that was inserted in the Sketch
	var cnt uint
	for _, v := range keyCounts {
		cnt += v.n
	}
	sum := inputSketch.GetSum()
	avg := sum / float64(cnt)
	max, err := inputSketch.GetMaxValue()
	if err != nil {
		return nil, fmt.Errorf("couldn't compute maximum of ddsketch: %w", err)
	}

	min, err := inputSketch.GetMinValue()
	if err != nil {
		return nil, fmt.Errorf("couldn't compute minimum of ddsketch: %w", err)
	}

	summary := summary.Summary{
		Cnt: int64(cnt),
		Sum: sum,
		Avg: avg,
		Max: max,
		Min: min,
	}

	// Build the final Sketch object
	outputSketch := &Sketch{
		sparseStore: sparseStore,
		Basic:       summary,
	}

	return outputSketch, nil
}

// FromDDSketch converts a DDSketch into a Sketch
func FromDDSketch(inputSketch *ddsketch.DDSketch) (*Sketch, error) {
	sketchConfig := Default()

	compatibleDDSketch, err := convertToCompatibleDDSketch(sketchConfig, inputSketch)
	if err != nil {
		return nil, fmt.Errorf("couldn't convert input ddsketch into ddsketch with compatible parameters: %w", err)
	}

	outputSketch, err := fromCompatibleDDSketch(sketchConfig, compatibleDDSketch)
	if err != nil {
		return nil, fmt.Errorf("couldn't convert ddsketch into Sketch: %w", err)
	}

	return outputSketch, nil
}
