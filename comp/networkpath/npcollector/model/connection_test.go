// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewBaselineSignals(t *testing.T) {
	for name, counts := range map[string][3]uint64{
		"healthy":    {},
		"timeout":    {1, 0, 0},
		"rto":        {0, 1, 0},
		"retransmit": {0, 0, 1},
	} {
		t.Run(name, func(t *testing.T) {
			signals := NewBaselineSignals(counts[0], counts[1], counts[2], 5, 6)

			assert.Equal(t, name != "healthy", signals.Diagnostic)
			assert.Equal(t, uint64(11), signals.Bytes)
		})
	}
}
