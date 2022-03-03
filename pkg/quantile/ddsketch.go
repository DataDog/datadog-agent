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
	offset := float64(c.norm.bias)
	newMapping, err := mapping.NewLogarithmicMappingWithGamma(gamma, offset)
	if err != nil {
		return nil, fmt.Errorf("couldn't create LogarithmicMapping for DDSketch: %v", err)
	}

	newSketch := inputSketch.ChangeMapping(newMapping, positiveStore, negativeStore, 1.0)
	return newSketch, nil
}

type floatKeyCount struct {
	k int
	c float64
}

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

	var loopErr error

	floatKeyCounts := make([]floatKeyCount, 0, defaultBinListSize)
	inputSketch.ForEach(func(value, count float64) bool {
		if count <= 0 {
			loopErr = fmt.Errorf("negative counts are not supported: got %f", count)
			return true
		}

		// The value passed to Index must be positive, so we
		// store the sign in a separate variable, in order to
		// reuse it when computing the sparseStore key associated to the value
		// (since negative values get mapped to negative indexes).
		sign := 1
		if value < 0 {
			sign = -1
			value = -value
		}

		var index int
		if value == 0 {
			index = 0
		} else {
			index = inputSketch.Index(value)
		}

		if index >= maxIndex {
			loopErr = fmt.Errorf("index value %d exceeds the maximum supported index value (%d)", index, maxIndex)
			return true
		}

		// All indexes <= 0 represent values that are <= minValue,
		// and therefore end up in the zero bin
		if index <= 0 {
			// TODO: What if zeroes overflows float64?
			zeroes += count
			return false
		}

		floatKeyCounts = append(floatKeyCounts, floatKeyCount{k: sign * index, c: count})
		return false
	})

	if loopErr != nil {
		return nil, loopErr
	}

	// Finally, add the 0 key
	floatKeyCounts = append(floatKeyCounts, floatKeyCount{k: 0, c: zeroes})

	keyCounts := keyCountsFromFloatKeyCounts(floatKeyCounts)

	// Populate sparseStore object with the collected keyCounts
	sparseStore.insertCounts(c, keyCounts)

	// Create summary object
	// Calculate the total count that was inserted in the Sketch
	var cnt uint
	for _, v := range keyCounts {
		cnt += v.n
	}
	sum := inputSketch.GetSum()
	avg := sum / float64(cnt)
	max, err := inputSketch.GetValueAtQuantile(1.0)
	if err != nil {
		return nil, fmt.Errorf("couldn't compute maximum of ddsketch: %v", err)
	}

	min, err := inputSketch.GetValueAtQuantile(0.0)
	if err != nil {
		return nil, fmt.Errorf("couldn't compute minimum of ddsketch: %v", err)
	}

	summary := summary.Summary{
		Cnt: int64(cnt),
		Sum: sum,
		Avg: avg,
		Max: max,
		Min: min,
	}

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
		return nil, fmt.Errorf("couldn't convert input ddsketch into ddsketch with compatible parameters: %v", err)
	}

	outputSketch, err := fromCompatibleDDSketch(sketchConfig, compatibleDDSketch)
	if err != nil {
		return nil, fmt.Errorf("couldn't convert ddsketch into Sketch: %v", err)
	}

	return outputSketch, nil
}
