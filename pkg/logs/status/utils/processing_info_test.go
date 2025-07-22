// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProcessingInfo(t *testing.T) {
	info := NewProcessingInfo()

	// Test initial state
	assert.Equal(t, "Processing Rules", info.InfoKey())
	assert.Equal(t, []string{"No rules applied"}, info.Info())
	assert.Equal(t, int64(0), info.GetCount("nonexistent"))

	// Test incrementing rules
	info.Inc("exclude_rule")
	assert.Equal(t, int64(1), info.GetCount("exclude_rule"))

	info.Inc("exclude_rule")
	assert.Equal(t, int64(2), info.GetCount("exclude_rule"))

	info.Inc("mask_rule")
	assert.Equal(t, int64(1), info.GetCount("mask_rule"))

	// Test Info() output
	infoStrings := info.Info()
	expected := []string{"Rule [exclude_rule] applied to 2 log(s)", "Rule [mask_rule] applied to 1 log(s)"}
	assert.Equal(t, expected, infoStrings)
}
