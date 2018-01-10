// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package docker

import (
	"fmt"
	"testing"
)

func init() {
	registerComposeFile("basemetrics.compose")
}

func TestContainerMetricsTagging(t *testing.T) {
	expectedTags := []string{
		instanceTag,                                                         // Instance tags
		"container_name:basemetrics_redis_1",                                // Container name
		"docker_image:redis:latest", "image_name:redis", "image_tag:latest", // Image tags
		"highcardlabeltag:redishigh", "lowcardlabeltag:redislow", // Labels as tags
		"highcardenvtag:redishighenv", "lowcardenvtag:redislowenv", // Env as tags
	}

	expectedMetrics := map[string][]string{
		"Gauge": {"docker.mem.cache", "docker.mem.rss",
			"docker.container.size_rw", "docker.container.size_rootfs"},
		"Rate": {"docker.cpu.system", "docker.cpu.user", "docker.cpu.usage", "docker.cpu.throttled",
			"docker.io.read_bytes", "docker.io.write_bytes",
			"docker.net.bytes_sent", "docker.net.bytes_rcvd"},
	}

	for method, metricList := range expectedMetrics {
		for _, metric := range metricList {
			found := sender.AssertMetricTaggedWith(t, method, metric, expectedTags)
			if !found {
				fmt.Printf("Missing %s %s with tags %s\n", method, metric, expectedTags)
			}
		}
	}
}
