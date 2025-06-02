// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package slices

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMap(t *testing.T) {
	x := Map([]int{1, 2, 4, 8}, func(v int) int {
		return v * v
	})
	assert.Equal(t, []int{1, 4, 16, 64}, x)
}
