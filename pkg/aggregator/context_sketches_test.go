// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContextSketchesRejectNonFiniteValues(t *testing.T) {
	m := make(contextSketches)

	assert.False(t, m.insert(1, math.NaN(), 1))
	assert.False(t, m.insert(1, math.Inf(1), 1))
	assert.False(t, m.insertInterp(1, math.Inf(-1), 2, 3))
	assert.False(t, m.insertInterp(1, 1, math.NaN(), 3))
	assert.Empty(t, m, "no sketch is created for a value it can't hold")

	assert.True(t, m.insert(1, 1, 1))
	assert.True(t, m.insertInterp(1, 1, 2, 3))
	assert.Equal(t, int64(4), m[1].Finish().Basic.Cnt)
}
