// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package docker

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func init() {
	registerComposeFile("events.compose")
}

func TestEvents(t *testing.T) {
	nowTimestamp := time.Now().Unix()
	expectedTags := []string{
		instanceTag,
		"highcardlabeltag:eventhigh",
		"lowcardlabeltag:eventlow",
		"highcardenvtag:eventhighenv",
		"lowcardenvtag:eventlowenv",
	}

	expectedBusyboxEvent := metrics.Event{
		Ts:        nowTimestamp,
		EventType: "docker",
		Tags: append(expectedTags, []string{
			"docker_image:datadog/docker-library:busybox_1_28_0",
			"image_name:datadog/docker-library",
			"short_image:docker-library",
			"image_tag:busybox_1_28_0",
			"container_name:events_recordingevent0_1",
			"container_name:events_recordingevent1_1",
		}...),
		AggregationKey: "docker:datadog/docker-library:busybox_1_28_0",
		SourceTypeName: "docker",
		Priority:       metrics.EventPriorityNormal,
	}
	sender.AssertEvent(t, expectedBusyboxEvent, time.Minute)

	expectedRedisEvent := metrics.Event{
		Ts:        nowTimestamp,
		EventType: "docker",
		Tags: append(expectedTags, []string{
			"docker_image:datadog/docker-library:redis_3_2_11-alpine",
			"image_name:datadog/docker-library",
			"short_image:docker-library",
			"image_tag:redis_3_2_11-alpine",
			"container_name:events_recordingevent2_1",
		}...),
		AggregationKey: "docker:datadog/docker-library:redis_3_2_11-alpine",
		SourceTypeName: "docker",
		Priority:       metrics.EventPriorityNormal,
	}
	sender.AssertEvent(t, expectedRedisEvent, time.Minute)

	// Put the expected expectedRedisEvent event in the future
	expectedRedisEvent.Ts = time.Now().Unix() + 60
	sender.AssertEventMissing(t, expectedRedisEvent, time.Second) // Allow a delta of 1 second
}
