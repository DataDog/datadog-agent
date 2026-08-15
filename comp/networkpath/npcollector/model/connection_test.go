// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package model

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetBaselineSignals(t *testing.T) {
	var conn NetworkPathConnection
	conn.SetBaselineSignals(1, 2, 3, 4, math.MaxUint64, 1)

	assert.True(t, conn.TCPTimeout)
	assert.True(t, conn.TCPRTO)
	assert.Equal(t, uint64(3), conn.Retransmits)
	assert.Equal(t, uint64(4), conn.RTTVar)
	assert.Equal(t, uint64(math.MaxUint64), conn.Bytes)
	assert.True(t, conn.NumericSaturated)
}
