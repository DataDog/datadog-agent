// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package containers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

func TestReportExitCodes(t *testing.T) {
	dockerCheck := &DockerCheck{
		instance: &DockerConfig{},
	}
	mockSender := mocksender.NewMockSender(dockerCheck.ID())

	var events []*docker.ContainerEvent

	// Don't fail on empty event array
	err := dockerCheck.reportExitCodes(events, mockSender)
	assert.Nil(t, err)
	mockSender.AssertNumberOfCalls(t, "ServiceCheck", 0)

	// Gracefully skip invalid events
	events = append(events, &docker.ContainerEvent{})
	events = append(events, &docker.ContainerEvent{
		Action: "die",
	})
	events = append(events, &docker.ContainerEvent{
		Action:     "die",
		Attributes: map[string]string{"exitCode": "nonNumeric"},
	})
	err = dockerCheck.reportExitCodes(events, mockSender)
	assert.Nil(t, err)
	mockSender.AssertNumberOfCalls(t, "ServiceCheck", 0)

	// Reset event array
	events = events[0:0]

	// Valid exit 0 event
	events = append(events, &docker.ContainerEvent{
		Action:        "die",
		Attributes:    map[string]string{"exitCode": "0"},
		ContainerID:   "fcc487ac70446287ae0dc79fb72368d824ff6198cd1166a405bc5a7fc111d3a8",
		ContainerName: "goodOne",
	})
	mockSender.On("ServiceCheck", "docker.exit", metrics.ServiceCheckOK, "",
		mock.AnythingOfType("[]string"), "Container goodOne exited with 0")

	// Valid exit 1 event
	events = append(events, &docker.ContainerEvent{
		Action:        "die",
		Attributes:    map[string]string{"exitCode": "1"},
		ContainerID:   "fcc487ac70446287ae0dc79fb72368d824ff6198cd1166a405bc5a7fc111d3a8",
		ContainerName: "badOne",
	})
	mockSender.On("ServiceCheck", "docker.exit", metrics.ServiceCheckCritical, "",
		mock.AnythingOfType("[]string"), "Container badOne exited with 1")

	err = dockerCheck.reportExitCodes(events, mockSender)
	assert.Nil(t, err)
	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "ServiceCheck", 2)
}

func TestAggregateEvents(t *testing.T) {
	testCases := []struct {
		events          []*docker.ContainerEvent
		filteredActions []string
		output          map[string]*dockerEventBundle
	}{
		{
			events:          nil,
			filteredActions: nil,
			output:          make(map[string]*dockerEventBundle),
		},
		{
			// One filtered out, and one not filtered
			events: []*docker.ContainerEvent{
				{
					ImageName: "test_image",
					Action:    "unfiltered_action",
				},
				{
					ImageName: "test_image",
					Action:    "top",
				},
			},
			filteredActions: []string{"top", "exec_create", "exec_start"},
			output: map[string]*dockerEventBundle{
				"test_image": {
					imageName: "test_image",
					countByAction: map[string]int{
						"unfiltered_action": 1,
					},
				},
			},
		},
		{
			// Only one filtered out action, empty output
			events: []*docker.ContainerEvent{
				{
					ImageName: "test_image",
					Action:    "top",
				},
			},
			filteredActions: []string{"top", "exec_create", "exec_start"},
			output:          map[string]*dockerEventBundle{},
		},
		{
			// 2+1 events, to count correctly
			events: []*docker.ContainerEvent{
				{
					ImageName: "test_image",
					Action:    "unfiltered_action",
				},
				{
					ImageName: "test_image",
					Action:    "unfiltered_action",
				},
				{
					ImageName: "test_image",
					Action:    "other_action",
				},
			},
			filteredActions: []string{"top", "exec_create", "exec_start"},
			output: map[string]*dockerEventBundle{
				"test_image": {
					imageName: "test_image",
					countByAction: map[string]int{
						"unfiltered_action": 2,
						"other_action":      1,
					},
				},
			},
		},
		{
			// Two images
			events: []*docker.ContainerEvent{
				{
					ImageName: "test_image",
					Action:    "unfiltered_action",
				},
				{
					ImageName: "test_image",
					Action:    "unfiltered_action",
				},
				{
					ImageName: "test_image",
					Action:    "other_action",
				},
				{
					ImageName: "other_image",
					Action:    "other_action",
				},
			},
			filteredActions: []string{"top", "exec_create", "exec_start"},
			output: map[string]*dockerEventBundle{
				"test_image": {
					imageName: "test_image",
					countByAction: map[string]int{
						"unfiltered_action": 2,
						"other_action":      1,
					},
				},
				"other_image": {
					imageName: "other_image",
					countByAction: map[string]int{
						"other_action": 1,
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			bundles := aggregateEvents(tc.events, tc.filteredActions)
			for _, b := range bundles {
				// Strip underlying events to ease testing
				// countByAction is enough for testing the
				// filtering and aggregation
				b.events = nil
			}
			assert.EqualValues(t, tc.output, bundles)
		})
	}
}
