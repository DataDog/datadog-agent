// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agentstackmonitorimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRingFillAndWrap(t *testing.T) {
	var r ring[int]
	for i := 1; i <= bufferSize; i++ {
		r.push(i)
	}
	assert.Equal(t, bufferSize, r.filled)
	assert.Equal(t, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, r.values())

	// Wrap: push three more, oldest three should drop off.
	r.push(11)
	r.push(12)
	r.push(13)
	assert.Equal(t, bufferSize, r.filled)
	assert.Equal(t, []int{4, 5, 6, 7, 8, 9, 10, 11, 12, 13}, r.values())
}

func TestRingCountMatching(t *testing.T) {
	var r ring[float64]
	// Six of these ten values exceed 0.9.
	for _, v := range []float64{0.5, 0.6, 0.95, 0.95, 0.4, 0.99, 0.95, 0.8, 0.95, 0.95} {
		r.push(v)
	}
	assert.Equal(t, 6, r.countMatching(func(v float64) bool { return v > 0.9 }))
}

func TestSumInt(t *testing.T) {
	var r ring[int32]
	assert.EqualValues(t, 0, sumInt(&r), "empty ring")

	for _, v := range []int32{0, 1, 0, 2, 0} {
		r.push(v)
	}
	assert.EqualValues(t, 3, sumInt(&r))
}
