// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetBaselineSignals(t *testing.T) {
	for _, counts := range [][2]uint64{{1, 0}, {0, 1}} {
		var conn NetworkPathConnection
		conn.SetBaselineSignals(counts[0], counts[1], 3, 5, 6)

		assert.True(t, conn.TimeoutOrRTO)
		assert.Equal(t, uint64(3), conn.Retransmits)
		assert.Equal(t, uint64(11), conn.Bytes)
	}
}
