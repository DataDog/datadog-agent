// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringInRuneset(t *testing.T) {
	for nb, tc := range []struct {
		data     string
		runeset  string
		expected bool
	}{
		{
			data:     "15467892345",
			runeset:  "123456789",
			expected: true,
		},
		{
			data:     "015467892345",
			runeset:  "123456789",
			expected: false,
		},
		{
			data:     "15467892345g",
			runeset:  "123456789",
			expected: false,
		},
	} {
		t.Run(fmt.Sprintf("case %d: %s", nb, tc.data), func(t *testing.T) {
			assert.Equal(t, tc.expected, StringInRuneset(tc.data, tc.runeset))
		})
	}
}
