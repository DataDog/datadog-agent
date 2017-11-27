// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package docker

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

func init() {
	registerComposeFile("globalmetrics.compose")
}

func TestGlobalMetrics(t *testing.T) {
	expectedTags := []string{instanceTag}

	sender.AssertCalled(t, "Gauge", "docker.images.available", mocksender.IsGreaterOrEqual(2), "", mocksender.MatchTagsContains(expectedTags))
	sender.AssertCalled(t, "Gauge", "docker.images.intermediate", mocksender.IsGreaterOrEqual(0), "", mocksender.MatchTagsContains(expectedTags))

	redisTags := append(expectedTags, []string{"docker_image:redis:latest", "image_name:redis", "image_tag:latest"}...)
	sender.AssertCalled(t, "Gauge", "docker.containers.running", mocksender.IsGreaterOrEqual(1), "", mocksender.MatchTagsContains(redisTags))

	buxyboxTags := append(expectedTags, []string{"docker_image:busybox:latest", "image_name:busybox", "image_tag:latest"}...)
	sender.AssertCalled(t, "Gauge", "docker.containers.stopped", mocksender.IsGreaterOrEqual(1), "", mocksender.MatchTagsContains(buxyboxTags))
}
