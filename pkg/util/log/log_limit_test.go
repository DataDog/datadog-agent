// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLogLimit(t *testing.T) {
	interval := 10 * time.Millisecond
	l := NewLogLimit(10, interval)

	for i := 0; i < 10; i++ {
		assert.True(t, l.ShouldLog())
	}

	assert.False(t, l.ShouldLog())
	assert.False(t, l.ShouldLog())

	time.Sleep(10 * time.Millisecond)

	assert.True(t, l.ShouldLog())
	assert.False(t, l.ShouldLog())
}
