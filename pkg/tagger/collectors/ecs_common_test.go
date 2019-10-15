// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package collectors

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/stretchr/testify/assert"
)

func TestAddTaskResourceTags(t *testing.T) {
	tags := utils.NewTagList()
	taskTags := map[string]string{
		"environment":         "sandbox",
		"project":             "ecs-test",
		"aws:ecs:clusterName": "test-cluster",
		"aws:ecs:serviceName": "nginx-awsvpc",
	}

	expectedTags := utils.NewTagList()
	expectedTags.AddLow("environment", "sandbox")
	expectedTags.AddLow("project", "ecs-test")

	addResourceTags(tags, taskTags)

	assert.Equal(t, expectedTags, tags)
}
