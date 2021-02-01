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
		reqTags    []string
		rejectTags []string
		traceMeta  map[string]string
		isValid    bool
	}{
		{
			[]string{"important1", "important2"},
			[]string{},
			map[string]string{
				"important1": "test-value",
				"important2": "test-value-2",
			},
			true,
		},
		{
			[]string{"important1", "important2"},
			[]string{},
			map[string]string{
				"important1": "test-value",
				"important2": "another-test-value",
				"blah":       "blah",
			},
			true,
		},
		{
			[]string{"important1", "important2"},
			[]string{},
			map[string]string{
				"important1": "test-value",
				"blah":       "blah",
			},
			false,
		},
		{
			[]string{},
			[]string{"reject1"},
			map[string]string{
				"somekey": "12345",
				"blah":    "blah",
			},
			true,
		},
		{
			[]string{},
			[]string{"reject1"},
			map[string]string{
				"somekey": "12345",
				"reject1": "bad",
			},
			false,
		},
		{
			[]string{"important1"},
			[]string{"reject1", "reject2"},
			map[string]string{
				"important1": "test-value",
				"reject1":    "bad",
				"reject2":    "also-bad",
			},
			false,
		},
	}

	for _, test := range tests {
		span := testutil.RandomSpan()
		span.Meta = test.traceMeta
		filter := NewValidator(test.reqTags, test.rejectTags)

		assert.Equal(t, test.isValid, filter.Validates(span))
	}
}
