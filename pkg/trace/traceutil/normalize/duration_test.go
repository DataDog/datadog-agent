// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package normalize

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFixDuration(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		fixed, invalid := FixDuration(time.Now().UnixNano(), 200000000)
		assert.False(t, invalid)
		assert.EqualValues(t, 200000000, fixed)
	})

	t.Run("negative", func(t *testing.T) {
		fixed, invalid := FixDuration(time.Now().UnixNano(), -50)
		assert.True(t, invalid)
		assert.EqualValues(t, 0, fixed)
	})

	t.Run("overflow", func(t *testing.T) {
		fixed, invalid := FixDuration(time.Now().UnixNano(), math.MaxInt64)
		assert.True(t, invalid)
		assert.EqualValues(t, 0, fixed)
	})
}

func TestFixStartTime(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		start := time.Now().UnixNano()
		fixed, invalid := FixStartTime(start, 200000000)
		assert.False(t, invalid)
		assert.EqualValues(t, start, fixed)
	})

	t.Run("too small", func(t *testing.T) {
		minStart := time.Now().UnixNano()
		fixed, invalid := FixStartTime(42, 200000000)
		assert.True(t, invalid)
		assert.GreaterOrEqual(t, fixed, minStart-200000000)
		assert.LessOrEqual(t, fixed, time.Now().UnixNano()-200000000)
	})

	t.Run("too small with large duration", func(t *testing.T) {
		minStart := time.Now().UnixNano()
		fixed, invalid := FixStartTime(42, time.Now().UnixNano()*2)
		assert.True(t, invalid)
		assert.GreaterOrEqual(t, fixed, minStart)
		assert.LessOrEqual(t, fixed, time.Now().UnixNano())
	})
}
