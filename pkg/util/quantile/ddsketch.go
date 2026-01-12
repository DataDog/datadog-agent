// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package quantile

import (
	"fmt"
	"math"
	"sort"

	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/DataDog/sketches-go/ddsketch/mapping"
	"github.com/DataDog/sketches-go/ddsketch/store"

	"github.com/DataDog/datadog-agent/pkg/util/quantile/summary"
)

const (
	maxIndex = math.MaxInt16
)

// createDDSketchWithSketchMapping takes a DDSketch and returns a new DDSketch
// with a logarithmic mapping that matches the Sketch parameters.
func createDDSketchWithSketchMapping(c *Config, inputSketch *ddsketch.DDSketch) (*ddsketch.DDSketch, error) {
	// Create positive store for the new DDSketch
	positiveStore := getDenseStore()

	// Create negative store for the new DDSketch
	negativeStore := getDenseStore()

	// Take parameters that match the Sketch mapping, and create a LogarithmicMapping out of them
	gamma := c.gamma.v
	// Note: there's a 0.5 shift here because we take the floor value on DDSketch, vs. rounding to
	// integer in the Agent sketch.
	offset := float64(c.norm.bias) + 0.5
	newMapping, err := mapping.NewLogarithmicMappingWithGamma(gamma, offset)
	if err != nil {
		// We don't use defer here because in the normal path
		// we pass ownership of the stores to ConvertDDSketchIntoSketch.
		putDenseStore(positiveStore)
		putDenseStore(negativeStore)
		return nil, fmt.Errorf("couldn't create LogarithmicMapping for DDSketch: %w", err)
	}

	if inputSketch.GetCount() == 1.0 {
		// We know the exact value of this one point: it is the sum.
		// Avoid remapping artifacts in that special case.
		newSketch := ddsketch.NewDDSketch(newMapping, positiveStore, negativeStore)
		if err := newSketch.Add(inputSketch.GetSum()); err == nil {
			return newSketch, nil
		}
	}

	newSketch := inputSketch.ChangeMapping(newMapping, positiveStore, negativeStore, 1.0)
	return newSketch, nil
}

type floatKeyCount struct {
	k int
	c float64
}

// convertFloatCountsToIntCounts converts a list of float counts to integer counts,
// preserving the total count of the list by tracking leftover decimal counts.
func convertFloatCountsToIntCounts(floatKeyCounts []floatKeyCount) ([]KeyCount, uint) {
	keyCounts := getKeyCountList()
	defer putKeyCountList(keyCounts)

	sort.Slice(floatKeyCounts, func(i, j int) bool {
		return floatKeyCounts[i].k < floatKeyCounts[j].k
	})

	floatTotal := 0.0
	intTotal := uint(0)
	for _, fkc := range floatKeyCounts {
		floatTotal += fkc.c
		rounded := uint(math.Round(floatTotal)) - intTotal
		intTotal += rounded
		// At this point, intTotal == Round(floatTotal)
		if rounded > 0 {
			keyCounts = append(keyCounts, KeyCount{k: Key(fkc.k), n: rounded})
		}
	}

	// Create a copy of the result since we're returning the pooled slice
	result := make([]KeyCount, len(keyCounts))
	copy(result, keyCounts)

	return result, intTotal
}

// convertDDSketchIntoSketch takes a DDSketch and moves its data to a Sketch.
// The conversion assumes that the DDSketch has a mapping that is compatible
// with the Sketch parameters (eg. a DDSketch returned by convertDDSketchMapping).
func convertDDSketchIntoSketch(c *Config, inputSketch *ddsketch.DDSketch) (*Sketch, error) {
	// Get the bin list from pool
	bins := getBinList()
	defer putBinList(bins)

	sparseStore := sparseStore{
		bins:  bins,
		count: 0,
	}

	// Special counter to aggregate all zeroes
	zeroes := 0.0

	// Get float key counts from pool
	floatKeyCounts := getFloatKeyCountList()
	defer putFloatKeyCountList(floatKeyCounts)

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

	zeroes += inputSketch.GetZeroCount()

	// Finally, add the 0 key
	if zeroes != 0 {
		floatKeyCounts = append(floatKeyCounts, floatKeyCount{k: 0, c: zeroes})
	}

	// Generate the integer KeyCount objects from the counts we retrieved
	keyCounts, cnt := convertFloatCountsToIntCounts(floatKeyCounts)

	// Populate sparseStore object with the collected keyCounts
	// insertCounts will take care of creating multiple uint16 bins for a
	// single key if the count overflows uint16
	sparseStore.insertCounts(c, keyCounts)

	// Create summary object
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

// ConvertDDSketchIntoSketch converts a DDSketch into a Sketch, by first
// converting the DDSketch into a new DDSketch with a mapping that's compatible
// with Sketch parameters, then creating the Sketch by copying the DDSketch
// bins to the Sketch store.
func ConvertDDSketchIntoSketch(inputSketch *ddsketch.DDSketch) (*Sketch, error) {
	sketchConfig := Default()

	compatibleDDSketch, err := createDDSketchWithSketchMapping(sketchConfig, inputSketch)
	if err != nil {
		return nil, fmt.Errorf("couldn't convert input ddsketch into ddsketch with compatible parameters: %w", err)
	}

	outputSketch, err := convertDDSketchIntoSketch(sketchConfig, compatibleDDSketch)
	if err != nil {
		return nil, fmt.Errorf("couldn't convert ddsketch into Sketch: %w", err)
	}

	return outputSketch, nil
}
