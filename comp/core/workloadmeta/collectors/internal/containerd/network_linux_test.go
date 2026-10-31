// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd && linux

package containerd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSelectIP(t *testing.T) {
	tests := []struct {
		name     string
		ips      []string
		expected string
	}{
		{
			name:     "single IP",
			ips:      []string{"10.0.0.7"},
			expected: "10.0.0.7",
		},
		{
			name:     "ECS awsvpc task prefers the ENI IP",
			ips:      []string{"169.254.172.3", "172.31.40.12"},
			expected: "172.31.40.12",
		},
		{
			name:     "only link-local IPs",
			ips:      []string{"169.254.172.3", "169.254.172.4"},
			expected: "169.254.172.3",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, selectIP(test.ips))
		})
	}
}
