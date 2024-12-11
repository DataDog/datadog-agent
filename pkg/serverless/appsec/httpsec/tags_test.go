// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package httpsec

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeHTTPHeaders(t *testing.T) {
	for _, tc := range []struct {
		headers  map[string][]string
		expected map[string]string
	}{
		{
			headers:  nil,
			expected: nil,
		},
		{
			headers: map[string][]string{
				"cookie": {"not-collected"},
			},
			expected: nil,
		},
		{
			headers: map[string][]string{
				"cookie":          {"not-collected"},
				"x-forwarded-for": {"1.2.3.4,5.6.7.8"},
			},
			expected: map[string]string{
				"x-forwarded-for": "1.2.3.4,5.6.7.8",
			},
		},
		{
			headers: map[string][]string{
				"cookie":          {"not-collected"},
				"x-forwarded-for": {"1.2.3.4,5.6.7.8", "9.10.11.12,13.14.15.16"},
			},
			expected: map[string]string{
				"x-forwarded-for": "1.2.3.4,5.6.7.8,9.10.11.12,13.14.15.16",
			},
		},
	} {
		headers := normalizeHTTPHeaders(tc.headers)
		require.Equal(t, tc.expected, headers)
	}
}
