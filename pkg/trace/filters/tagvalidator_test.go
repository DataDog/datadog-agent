// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package filters

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
	"github.com/stretchr/testify/assert"
)

func TestValidator(t *testing.T) {
	tests := []struct {
		tagRules    []*config.TagRule
		traceMeta   map[string]string
		expectError error
	}{
		{
			tagRules: []*config.TagRule{
				{Type: 1, Name: "important1"},
				{Type: 1, Name: "important2"},
			},
			traceMeta: map[string]string{
				"important1": "test-value",
				"important2": "test-value-2",
			},
		},
		{
			tagRules: []*config.TagRule{
				{Type: 1, Name: "important1", Value: "value:subvalue"},
				{Type: 1, Name: "important2"},
			},
			traceMeta: map[string]string{
				"important1": "value:subvalue",
				"important2": "test-value-2",
			},
		},
		{
			tagRules: []*config.TagRule{
				{Type: 1, Name: "test1", Value: "test-value"},
				{Type: 1, Name: "test2", Value: "another-test-value"},
				{Type: 1, Name: "blah"},
			},
			traceMeta: map[string]string{
				"test1": "test-value",
				"test2": "another-test-value",
				"blah":  "blah",
			},
		},
		{
			tagRules: []*config.TagRule{
				{Type: 1, Name: "important1"},
				{Type: 1, Name: "important2"},
			},
			traceMeta: map[string]string{
				"important1": "test-value",
				"blah":       "blah",
			},
			expectError: errors.New(`required tag(s) missing`),
		},
		{
			tagRules: []*config.TagRule{
				{Type: 0, Name: "reject1"},
			},
			traceMeta: map[string]string{
				"somekey": "12345",
				"blah":    "blah",
			},
		},
		{
			tagRules: []*config.TagRule{
				{Type: 1, Name: "important1"},
				{Type: 0, Name: "reject1"},
			},
			traceMeta: map[string]string{
				"important1": "test-value",
				"reject1":    "bad",
				"reject2":    "also-bad",
			},
			expectError: errors.New(`invalid tag(s) found`),
		},
		{
			tagRules: []*config.TagRule{
				{Type: 1, Name: "important1", Value: "this-value-only"},
				{Type: 0, Name: "reject1"},
			},
			traceMeta: map[string]string{
				"important1": "test-value",
				"reject1":    "bad",
				"reject2":    "also-bad",
			},
			expectError: errors.New(`required tag(s) missing`),
		},
		{
			tagRules: []*config.TagRule{
				{Type: 1, Name: "important1", Value: "test-value"},
				{Type: 0, Name: "reject1", Value: "bad"},
				{Type: 0, Name: "reject2"},
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
		filter := NewTagValidator(test.tagRules)

		err := filter.Validates(span)
		if test.expectError != nil {
			assert.EqualError(t, err, test.expectError.Error())
		} else {
			assert.NoError(t, err)
		}
	}
}
