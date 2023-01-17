// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checkconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_normalizeRegexReplaceValue(t *testing.T) {
	tests := []struct {
		val                   string
		expectedReplacedValue string
	}{
		{
			"abc",
			"abc",
		},
		{
			"a\\1b",
			"a$1b",
		},
		{
			"a$1b",
			"a$1b",
		},
		{
			"\\1",
			"$1",
		},
		{
			"\\2",
			"$2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.val, func(t *testing.T) {
			assert.Equal(t, tt.expectedReplacedValue, normalizeRegexReplaceValue(tt.val))
		})
	}
}
