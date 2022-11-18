// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProcessNotify(t *testing.T) {
	pn := newProcessNotify()

	// Empty map on initialization
	assert.Equal(t, map[int32]int64{}, pn.GetCreateTimes([]int32{1, 2, 3}))

	createTimes := map[int32]int64{
		1: 1,
		2: 2,
		3: 3,
		4: 4,
	}

	// Test all existing PIDs
	pn.UpdateCreateTimes(createTimes)

	assert.Equal(t, createTimes, pn.GetCreateTimes([]int32{1, 2, 3, 4}))

	// Test non-existent PIDs
	expectedCreateTimes := map[int32]int64{
		1: 1,
		3: 3,
	}

	assert.Equal(t, expectedCreateTimes, pn.GetCreateTimes([]int32{1, 3, 5}))
}
