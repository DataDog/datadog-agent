// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taglist"
	"github.com/DataDog/datadog-agent/pkg/config"

	"github.com/stretchr/testify/assert"
)

func TestAddResourceTags(t *testing.T) {
	tests := []struct {
		name      string
		taskTags  map[string]string
		loadFunc  func() *taglist.TagList
		resetFunc func()
	}{
		{
			name: "nominal case",
			taskTags: map[string]string{
				"environment":         "sandbox",
				"project":             "ecs-test",
				"aws:ecs:clusterName": "test-cluster",
				"aws:ecs:serviceName": "nginx-awsvpc",
			},
			loadFunc: func() *taglist.TagList {
				expectedTags := taglist.NewTagList()
				expectedTags.AddLow("environment", "sandbox")
				expectedTags.AddLow("project", "ecs-test")
				return expectedTags
			},
			resetFunc: func() {},
		},
		{
			name: "replace colon enabled, replace tag key",
			taskTags: map[string]string{
				"environment":         "sandbox",
				"project":             "ecs-test",
				"foo:bar:baz":         "val",
				"aws:ecs:clusterName": "test-cluster",
				"aws:ecs:serviceName": "nginx-awsvpc",
			},
			loadFunc: func() *taglist.TagList {
				expectedTags := taglist.NewTagList()
				expectedTags.AddLow("environment", "sandbox")
				expectedTags.AddLow("project", "ecs-test")
				expectedTags.AddLow("foo_bar_baz", "val")
				config.Datadog.SetWithoutSource("ecs_resource_tags_replace_colon", true)
				return expectedTags
			},
			resetFunc: func() { config.Datadog.SetWithoutSource("ecs_resource_tags_replace_colon", false) },
		},
		{
			name: "replace colon enabled, do not replace tag value",
			taskTags: map[string]string{
				"environment":         "sandbox",
				"project":             "ecs-test",
				"foo:bar:baz":         "val1:val2",
				"aws:ecs:clusterName": "test-cluster",
				"aws:ecs:serviceName": "nginx-awsvpc",
			},
			loadFunc: func() *taglist.TagList {
				expectedTags := taglist.NewTagList()
				expectedTags.AddLow("environment", "sandbox")
				expectedTags.AddLow("project", "ecs-test")
				expectedTags.AddLow("foo_bar_baz", "val1:val2")
				config.Datadog.SetWithoutSource("ecs_resource_tags_replace_colon", true)
				return expectedTags
			},
			resetFunc: func() { config.Datadog.SetWithoutSource("ecs_resource_tags_replace_colon", false) },
		},
		{
			name: "replace colon disabled",
			taskTags: map[string]string{
				"environment":         "sandbox",
				"project":             "ecs-test",
				"foo:bar:baz":         "val",
				"aws:ecs:clusterName": "test-cluster",
				"aws:ecs:serviceName": "nginx-awsvpc",
			},
			loadFunc: func() *taglist.TagList {
				expectedTags := taglist.NewTagList()
				expectedTags.AddLow("environment", "sandbox")
				expectedTags.AddLow("project", "ecs-test")
				expectedTags.AddLow("foo:bar:baz", "val")
				return expectedTags
			},
			resetFunc: func() {},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer tt.resetFunc()
			tags := taglist.NewTagList()
			expectedTags := tt.loadFunc()
			addResourceTags(tags, tt.taskTags)
			assert.Equal(t, expectedTags, tags)
		})
	}
}
