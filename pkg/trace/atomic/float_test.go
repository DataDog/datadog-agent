// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package atomic

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFloat64(t *testing.T) {
	f := NewFloat(1.0)
	assert := assert.New(t)
	assert.Equal(f.Load(), 1.0)
	f.Store(2.0)
	assert.Equal(f.Load(), 2.0)
	assert.Equal(f.Swap(3.0), 2.0)
	assert.Equal(f.Load(), 3.0)
	assert.Equal(f.Add(1.0), 4.0)
	assert.Equal(f.Load(), 4.0)
	assert.Equal(f.Add(1.0), 5.0)
	assert.Equal(f.Load(), 5.0)
	assert.Equal(f.Sub(3.0), 2.0)
	assert.Equal(f.Load(), 2.0)
}
