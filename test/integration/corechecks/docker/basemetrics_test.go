// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package docker

import (
	"testing"

	log "github.com/cihub/seelog"
)

func init() {
	registerComposeFile("basemetrics.compose")
}

func TestContainerMetricsTagging(t *testing.T) {
	expectedTags := []string{
		instanceTag,                          // Instance tags
		"container_name:basemetrics_redis_1", // Container name
		"docker_image:datadog/docker-library:redis_3_2_11-alpine",
		"image_name:datadog/docker-library",
		"short_image:docker-library",
		"image_tag:redis_3_2_11-alpine",                          // Image tags
		"highcardlabeltag:redishigh", "lowcardlabeltag:redislow", // Labels as tags
		"highcardenvtag:redishighenv", "lowcardenvtag:redislowenv", // Env as tags
	}

	expectedMetrics := map[string][]string{
		"Gauge": {
			"docker.mem.cache",
			"docker.mem.rss",
			"docker.mem.in_use",
			"docker.mem.limit",
			"docker.container.size_rw",
			"docker.container.size_rootfs",
		},
		"Rate": {
			"docker.cpu.system",
			"docker.cpu.user",
			"docker.cpu.usage",
			"docker.cpu.throttled",
			"docker.io.read_bytes",
			"docker.io.write_bytes",
			"docker.net.bytes_sent",
			"docker.net.bytes_rcvd",
		},
	}
	pauseTags := []string{
		"instanceTag:MustBeHere",
		"docker_image:kubernetes/pause:latest",
		"image_name:kubernetes/pause",
		"image_tag:latest",
		"short_image:pause",
	}

	ok := sender.AssertMetricTaggedWith(t, "Gauge", "docker.containers.running", pauseTags)
	if !ok {
		log.Warnf("Missing Gauge docker.containers.running with tags %s", pauseTags)
	}

	for method, metricList := range expectedMetrics {
		for _, metric := range metricList {
			ok := sender.AssertMetricTaggedWith(t, method, metric, expectedTags)
			if !ok {
				log.Warnf("Missing %s %s with tags %s", method, metric, expectedTags)
			}

			// Excluded pause container
			ok = sender.AssertMetricNotTaggedWith(t, method, metric, pauseTags)
			if !ok {
				log.Warnf("Shouldn't call %s %s with tags %s", method, metric, pauseTags)
			}
		}
	}
}
