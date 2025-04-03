// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package payload

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTCPMethod(t *testing.T) {
	testcases := []struct {
		value  string
		parsed TCPMethod
	}{
		{"", TCPConfigSYN},
		{"syn", TCPConfigSYN},
		{"SYN", TCPConfigSYN},
		{"sack", TCPConfigSACK},
		{"SACK", TCPConfigSACK},
		{"prefer_sack", TCPConfigPreferSACK},
		{"prefer_SACK", TCPConfigPreferSACK},
		{"garbage", TCPConfigSYN},
	}
	for _, tc := range testcases {
		t.Run(fmt.Sprintf("parse '%s'", tc.value), func(t *testing.T) {
			parsed := ParseTCPMethod(tc.value)
			require.Equal(t, tc.parsed, parsed)
		})
	}
}
