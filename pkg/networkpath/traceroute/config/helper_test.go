// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPerHopTimeout(t *testing.T) {
	tests := []struct {
		name            string
		totalTimeout    time.Duration
		maxTTL          uint8
		expectedTimeout time.Duration
	}{
		{
			name:            "reserves ten percent",
			totalTimeout:    time.Second,
			maxTTL:          1,
			expectedTimeout: 900 * time.Millisecond,
		},
		{
			name:            "divides budget evenly across hops",
			totalTimeout:    time.Second,
			maxTTL:          30,
			expectedTimeout: 30 * time.Millisecond,
		},
		{
			name:            "preserves sub-millisecond precision",
			totalTimeout:    time.Millisecond,
			maxTTL:          30,
			expectedTimeout: 30 * time.Microsecond,
		},
		{
			name:            "maximum RC contract values",
			totalTimeout:    120 * time.Second,
			maxTTL:          255,
			expectedTimeout: 423*time.Millisecond + 529*time.Microsecond + 411*time.Nanosecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedTimeout, PerHopTimeout(tt.totalTimeout, tt.maxTTL))
		})
	}
}
