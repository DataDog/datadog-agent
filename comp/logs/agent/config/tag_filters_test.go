// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTagFiltersCompileNilReceiver(t *testing.T) {
	var f *TagFilters
	filters, report := f.Compile()
	assert.NotNil(t, filters)
	assert.True(t, filters.IsEmpty())
	assert.True(t, report.IsEmpty())
}

func TestTagFiltersCompile(t *testing.T) {
	f := &TagFilters{
		Include: []string{"pod_name:keep-me"},
		Exclude: []string{"container_id:*"},
	}
	filters, report := f.Compile()
	assert.NotNil(t, filters)
	assert.True(t, report.IsEmpty())
}
