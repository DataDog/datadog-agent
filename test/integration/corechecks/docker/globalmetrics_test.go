// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package docker

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

func init() {
	registerComposeFile("globalmetrics.compose")
}

func TestGlobalMetrics(t *testing.T) {
	expectedTags := []string{}

	sender.AssertCalled(t, "Gauge", "docker.images.available", mocksender.IsGreaterOrEqual(2), "", mocksender.MatchTagsContains(expectedTags))
	sender.AssertCalled(t, "Gauge", "docker.images.intermediate", mocksender.IsGreaterOrEqual(0), "", mocksender.MatchTagsContains(expectedTags))

	redisTags := append(expectedTags, []string{"docker_image:datadog/docker-library:redis_3_2_11-alpine", "image_name:datadog/docker-library", "image_tag:redis_3_2_11-alpine"}...)
	sender.AssertCalled(t, "Gauge", "docker.containers.running", mocksender.IsGreaterOrEqual(1), "", mocksender.MatchTagsContains(redisTags))

	buxyboxTags := append(expectedTags, []string{"docker_image:datadog/docker-library:busybox_1_28_0", "image_name:datadog/docker-library", "image_tag:busybox_1_28_0"}...)
	sender.AssertCalled(t, "Gauge", "docker.containers.stopped", mocksender.IsGreaterOrEqual(1), "", mocksender.MatchTagsContains(buxyboxTags))

	sender.AssertCalled(t, "Gauge", "docker.containers.running.total", mocksender.IsGreaterOrEqual(1), "", mocksender.MatchTagsContains(expectedTags))
	sender.AssertCalled(t, "Gauge", "docker.containers.stopped.total", mocksender.IsGreaterOrEqual(1), "", mocksender.MatchTagsContains(expectedTags))
}
