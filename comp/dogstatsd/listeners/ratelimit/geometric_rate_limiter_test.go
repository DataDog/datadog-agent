// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package ratelimit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeometricRateLimiterIncreaseDecrease(t *testing.T) {
	r := require.New(t)
	l := newGeometricRateLimiter(geometricRateLimiterConfig{
		minRate: 0.25,
		maxRate: 1.0,
		factor:  2,
	})
	r.Equal(0.25, l.currentRate())

	l.decreaseRate()
	r.Equal(0.25, l.currentRate())

	l.increaseRate()
	r.Equal(0.5, l.currentRate())

	l.increaseRate()
	r.Equal(1.0, l.currentRate())

	l.increaseRate()
	r.Equal(1.0, l.currentRate())

	l.decreaseRate()
	r.Equal(0.5, l.currentRate())
}

func TestGeometricRateLimiterLimitExceeded(t *testing.T) {
	r := require.New(t)
	l := newGeometricRateLimiter(geometricRateLimiterConfig{
		minRate: 0.25,
		maxRate: 1.0,
		factor:  2,
	})
	r.Equal(0.25, l.currentRate())
	r.False(l.keep())
	r.False(l.keep())
	r.False(l.keep())
	r.True(l.keep())

	l.increaseRate()
	r.Equal(0.5, l.currentRate())
	r.False(l.keep())
	r.True(l.keep())

	l.increaseRate()
	r.Equal(1.0, l.currentRate())
	r.True(l.keep())
}
