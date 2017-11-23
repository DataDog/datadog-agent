// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package docker

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/stretchr/testify/mock"
)

func init() {
	registerComposeFile("globalmetrics.compose")
}

func isGreaterOrEqual(atLeast float64) interface{} {
	return mock.MatchedBy(func(actual float64) bool {
		return atLeast <= actual
	})
}

func TestGlobalMetrics(t *testing.T) {
	expectedTags := []string{instanceTag}

	sender.AssertCalled(t, "Gauge", "docker.images.available", isGreaterOrEqual(2), "", mocksender.MatchTagsContains(expectedTags))
	sender.AssertCalled(t, "Gauge", "docker.images.intermediate", isGreaterOrEqual(0), "", mocksender.MatchTagsContains(expectedTags))

	redisTags := append(expectedTags, []string{"docker_image:redis:latest", "image_name:redis", "image_tag:latest"}...)
	sender.AssertCalled(t, "Gauge", "docker.containers.running", isGreaterOrEqual(1), "", mocksender.MatchTagsContains(redisTags))

	buxyboxTags := append(expectedTags, []string{"docker_image:busybox:latest", "image_name:busybox", "image_tag:latest"}...)
	sender.AssertCalled(t, "Gauge", "docker.containers.stopped", isGreaterOrEqual(1), "", mocksender.MatchTagsContains(buxyboxTags))
}
