// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package networkpath

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func Test_firstNonZero(t *testing.T) {
	tests := []struct {
		name          string
		values        []time.Duration
		expectedValue time.Duration
	}{
		{
			name:          "no value",
			expectedValue: 0,
		},
		{
			name: "one value",
			values: []time.Duration{
				time.Duration(10) * time.Second,
			},
			expectedValue: time.Duration(10) * time.Second,
		},
		{
			name: "multiple values - select first",
			values: []time.Duration{
				time.Duration(10) * time.Second,
				time.Duration(20) * time.Second,
				time.Duration(30) * time.Second,
			},
			expectedValue: time.Duration(10) * time.Second,
		},
		{
			name: "multiple values - select second",
			values: []time.Duration{
				time.Duration(0) * time.Second,
				time.Duration(20) * time.Second,
				time.Duration(30) * time.Second,
			},
			expectedValue: time.Duration(20) * time.Second,
		},
		{
			name: "multiple values - select third",
			values: []time.Duration{
				time.Duration(0) * time.Second,
				time.Duration(0) * time.Second,
				time.Duration(30) * time.Second,
			},
			expectedValue: time.Duration(30) * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedValue, firstNonZero(tt.values...))
		})
	}
}
