// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package filters

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
	"github.com/stretchr/testify/assert"
)

func TestValidator(t *testing.T) {
	tests := []struct {
		reqTags   []string
		traceMeta map[string]string
		isValid   bool
	}{
		{
			[]string{"important1", "important2"},
			map[string]string{
				"important1": "test-value",
				"important2": "test-value-2",
			},
			true,
		},
		{
			[]string{"important1", "important2"},
			map[string]string{
				"important1": "test-value",
				"important2": "another-test-value",
				"blah":       "blah",
			},
			true,
		},
		{
			[]string{"important1", "important2"},
			map[string]string{
				"important1": "test-value",
				"blah":       "blah",
			},
			false,
		},
		{
			[]string{},
			map[string]string{
				"somekey": "12345",
				"blah":    "blah",
			},
			true,
		},
		{
			[]string{},
			map[string]string{},
			true,
		},
	}

	for _, test := range tests {
		span := testutil.RandomSpan()
		span.Meta = test.traceMeta
		filter := NewValidator(test.reqTags)

		assert.Equal(t, test.isValid, filter.Validates(span))
	}
}
