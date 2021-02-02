// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package filters

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
	"github.com/stretchr/testify/assert"
)

func TestValidator(t *testing.T) {
	tests := []struct {
		reqTags     map[string]string
		rejectTags  map[string]string
		traceMeta   map[string]string
		expectError error
	}{
		{
			reqTags: map[string]string{
				"important1": "value",
				"important2": "value-2",
			},
			rejectTags: map[string]string{},
			traceMeta: map[string]string{
				"important1": "test-value",
				"important2": "test-value-2",
			},
		},
		{
			reqTags: map[string]string{
				"important1": "test-value",
				"important2": "another-test-value",
			},
			rejectTags: map[string]string{},
			traceMeta: map[string]string{
				"important1": "test-value",
				"important2": "another-test-value",
				"blah":       "blah",
			},
		},
		{
			reqTags: map[string]string{
				"important1": "test-value",
				"important2": "test-value",
			},
			rejectTags: map[string]string{},
			traceMeta: map[string]string{
				"important1": "test-value",
				"blah":       "blah",
			},
			expectError: errors.New(`required tag(s) missing`),
		},
		{
			reqTags: map[string]string{},
			rejectTags: map[string]string{
				"reject1": "bad-value",
			},
			traceMeta: map[string]string{
				"somekey": "12345",
				"blah":    "blah",
			},
		},
		{
			reqTags: map[string]string{},
			rejectTags: map[string]string{
				"reject1": "bad-value",
			},
			traceMeta: map[string]string{
				"somekey": "12345",
				"reject1": "bad",
			},
		},
		{
			reqTags: map[string]string{
				"important1": "value",
			},
			rejectTags: map[string]string{
				"reject1": "bad",
				"reject2": "also-bad",
			},
			traceMeta: map[string]string{
				"important1": "test-value",
				"reject1":    "bad",
				"reject2":    "also-bad",
			},
			expectError: errors.New(`invalid tag(s) found`),
		},
	}

	for _, test := range tests {
		span := testutil.RandomSpan()
		span.Meta = test.traceMeta
		filter := NewTagValidator(test.reqTags, test.rejectTags)

		err := filter.Validates(span)
		if test.expectError != nil {
			assert.EqualError(t, err, test.expectError.Error())
		} else {
			assert.NoError(t, err)
		}
	}
}
