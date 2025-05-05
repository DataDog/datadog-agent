// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetURL(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
		pids     []int32
		expected string
	}{
		{
			name:     "endpoint without pids",
			endpoint: "services",
			pids:     []int32{},
			expected: "http://sysprobe/discovery/services",
		},
		{
			name:     "endpoint with pids",
			endpoint: "language",
			pids:     []int32{1, 2, 3},
			expected: "http://sysprobe/discovery/language?pids=1%2C2%2C3",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			url := getDiscoveryURL(c.endpoint, c.pids)
			require.Equal(t, c.expected, url)
		})
	}
}
