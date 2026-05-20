// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMedianTimestampInterval(t *testing.T) {
	assert.Equal(t, int64(0), medianTimestampInterval(nil))
	assert.Equal(t, int64(0), medianTimestampInterval([]int64{10}))
	assert.Equal(t, int64(5), medianTimestampInterval([]int64{10, 15}))
	assert.Equal(t, int64(10), medianTimestampInterval([]int64{10, 20, 35, 45}))
	assert.Equal(t, int64(15), medianTimestampInterval([]int64{10, 40, 50, 65}))
}
