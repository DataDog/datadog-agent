// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package docker

import (
	"testing"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

func init() {
	registerComposeFile("basemetrics.compose")
}

func TestContainerMetricsTagging(t *testing.T) {
	expectedTags := []string{
		"container_name:basemetrics-redis-1", // Container name
		"docker_image:datadog/docker-library:redis_3_2_11-alpine",
		"image_name:datadog/docker-library",
		"short_image:docker-library",
		"image_tag:redis_3_2_11-alpine",                          // Image tags
		"highcardlabeltag:redishigh", "lowcardlabeltag:redislow", // Labels as tags
		"highcardenvtag:redishighenv", "lowcardenvtag:redislowenv", // Env as tags
	}

	expectedMetrics := map[string][]string{
		"Gauge": {
			"docker.cpu.shares",
			"docker.kmem.usage",
			"docker.mem.cache",
			"docker.mem.rss",
			"docker.mem.in_use",
			"docker.mem.limit",
			"docker.mem.failed_count",
			"docker.mem.soft_limit",
			"docker.container.open_fds",
			"docker.container.size_rw",
			"docker.container.size_rootfs",
			"docker.thread.count",
		},
		"Rate": {
			"docker.cpu.system",
			"docker.cpu.user",
			"docker.cpu.usage",
			"docker.cpu.throttled",
			// "docker.io.read_bytes", // With cgroupv2 the io.stat file is not filled if no IO has been made
			// "docker.io.write_bytes", // Our containers (redis) are not making any IO, thus the file is empty and metrics not generated
			"docker.net.bytes_sent",
			"docker.net.bytes_rcvd",
		},
	}
	pauseTags := []string{
		"docker_image:gcr.io/google_containers/pause:latest",
		"image_name:gcr.io/google_containers/pause",
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

	// redis:3.2 runs one process with 3 threads
	sender.AssertCalled(t, "Gauge", "docker.thread.count", 3.0, "", mocksender.MatchTagsContains(expectedTags))
	sender.AssertCalled(t, "Gauge", "docker.thread.limit", 25.0, "", mocksender.MatchTagsContains(expectedTags))
}
