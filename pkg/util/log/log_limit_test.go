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
	l := NewLogLimit(10, time.Hour)
	defer l.Close()

	for i := 0; i < 10; i++ {
		// this reset will not have any effect because we haven't logged 10 times yet
		l.resetCounter()
		assert.True(t, l.ShouldLog())
	}

	assert.False(t, l.ShouldLog())
	assert.False(t, l.ShouldLog())

	l.resetCounter()
	assert.True(t, l.ShouldLog())
	assert.False(t, l.ShouldLog())
}
