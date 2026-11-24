// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package filterlistimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics/metricname"
)

func TestNewTagMatcher(t *testing.T) {
	matcher := NewTagMatcher(map[string]MetricTagList{
		"metric1": {
			Tags:   []string{"env", "host"},
			Action: "exclude",
		},
		"metric2": {
			Tags:   []string{},
			Action: "include",
		},
		"metric3": {
			Tags:   []string{"pod"},
			Action: "invalid",
		},
	}, logmock.New(t))

	list, ok := matcher.ShouldStripTags("metric1")
	assert.True(t, ok)
	assert.Equal(t, metricname.NewHashedMetricTagList(true, []string{"env", "host"}), list)

	list, ok = matcher.ShouldStripTags("metric2")
	assert.True(t, ok)
	assert.Equal(t, metricname.NewHashedMetricTagList(false, []string{}), list)

	list, ok = matcher.ShouldStripTags("metric3")
	assert.True(t, ok)
	assert.Equal(t, metricname.NewHashedMetricTagList(true, []string{"pod"}), list)
}

func TestTagMatcher(t *testing.T) {
	metrics := map[string]MetricTagList{
		"metric1": {
			Tags:   []string{"env", "host"},
			Action: "exclude",
		},
		"metric2": {
			Tags:   []string{"env", "host"},
			Action: "include",
		},
		"metric3": {
			Tags:   []string{},
			Action: "include",
		},
		"metric4": {
			Tags:   []string{},
			Action: "exclude",
		},
	}

	matcher := NewTagMatcher(metrics, logmock.New(t))

	// Test metric1 tags are excluded
	tagList, shouldStrip := matcher.ShouldStripTags("metric1")
	assert.True(t, shouldStrip)

	assert.False(t, tagList.KeepTag("env:prod"))
	assert.False(t, tagList.KeepTag("host:server1"))
	assert.True(t, tagList.KeepTag("version:1.0"))

	// Test metric2 tags are included
	tagList, shouldStrip = matcher.ShouldStripTags("metric2")
	assert.True(t, shouldStrip)

	assert.True(t, tagList.KeepTag("env:prod"))
	assert.True(t, tagList.KeepTag("host:server1"))
	assert.False(t, tagList.KeepTag("version:1.0"))

	// Test metric3 tags are all excluded
	tagList, shouldStrip = matcher.ShouldStripTags("metric3")
	assert.True(t, shouldStrip)

	assert.False(t, tagList.KeepTag("env:prod"))
	assert.False(t, tagList.KeepTag("host:server1"))
	assert.False(t, tagList.KeepTag("version:1.0"))

	// Test metric4 tags are all included
	tagList, shouldStrip = matcher.ShouldStripTags("metric4")
	if shouldStrip { // 2 behaviors are acceptable: return true with a function that keeps all tags, or return false
		assert.True(t, tagList.KeepTag("env:prod"))
		assert.True(t, tagList.KeepTag("host:server1"))
		assert.True(t, tagList.KeepTag("version:1.0"))
	}

	// metric5 is not configured
	_, shouldStrip = matcher.ShouldStripTags("metric5")
	assert.False(t, shouldStrip)
}
