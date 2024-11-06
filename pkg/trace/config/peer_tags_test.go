// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPreparePeerTags(t *testing.T) {
	type testCase struct {
		input  []string
		output []string
	}

	for _, tc := range []testCase{
		{
			input:  nil,
			output: nil,
		},
		{
			input:  []string{},
			output: nil,
		},
		{
			input:  []string{"zz_tag", "peer.service", "some.other.tag", "db.name", "db.instance"},
			output: []string{"db.name", "db.instance", "peer.service", "some.other.tag", "zz_tag"},
		},
		{
			input:  append([]string{"zz_tag"}, basePeerTags...),
			output: append(basePeerTags, "zz_tag"),
		},
	} {
		sort.Strings(tc.output)
		assert.Equal(t, tc.output, preparePeerTags(tc.input))
	}
}

func TestDefaultPeerTags(t *testing.T) {
	// Simple test to ensure peer tags are loaded
	assert.Contains(t, basePeerTags, "db.name")
}
