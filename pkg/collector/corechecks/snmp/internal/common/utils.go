// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"
)

// CreateStringBatches batches strings into chunks with specific size
func CreateStringBatches(elements []string, size int) ([][]string, error) {
	var batches [][]string

	if size <= 0 {
		return nil, fmt.Errorf("batch size must be positive. invalid size: %d", size)
	}

	for i := 0; i < len(elements); i += size {
		j := i + size
		if j > len(elements) {
			j = len(elements)
		}
		batch := elements[i:j]
		batches = append(batches, batch)
	}

	return batches, nil
}

// AreTagsEqual compares two list of tags supposing that tags are in the same order
func AreTagsEqual(tagsA, tagsB []string) bool {
	if len(tagsA) != len(tagsB) {
		return false
	}

	for i, tag := range tagsA {
		if tag != tagsB[i] {
			return false
		}
	}

	return true
}
