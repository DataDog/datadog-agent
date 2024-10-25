// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"testing"

	"github.com/docker/docker/api/types/events"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

func TestReportExitCodes(t *testing.T) {
	fakeTagger := taggerimpl.SetupFakeTagger(t)

	dockerCheck := &DockerCheck{
		instance: &DockerConfig{},
		tagger:   fakeTagger,
	}

	dockerCheck.setOkExitCodes()
	mockSender := mocksender.NewMockSender(dockerCheck.ID())

	var events []*docker.ContainerEvent

	// Don't fail on empty event array
	dockerCheck.reportExitCodes(events, mockSender)
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
	dockerCheck.reportExitCodes(events, mockSender)
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
	mockSender.On("ServiceCheck", "docker.exit", servicecheck.ServiceCheckOK, "",
		[]string{"exit_code:0"}, "Container goodOne exited with 0")

	// Valid exit 143 event
	events = append(events, &docker.ContainerEvent{
		Action:        "die",
		Attributes:    map[string]string{"exitCode": "143"},
		ContainerID:   "fcc487ac70446287ae0dc79fb72368d824ff6198cd1166a405bc5a7fc111d3a8",
		ContainerName: "goodOne",
	})
	mockSender.On("ServiceCheck", "docker.exit", servicecheck.ServiceCheckOK, "",
		[]string{"exit_code:143"}, "Container goodOne exited with 143")

	// Valid exit 1 event
	events = append(events, &docker.ContainerEvent{
		Action:        "die",
		Attributes:    map[string]string{"exitCode": "1"},
		ContainerID:   "fcc487ac70446287ae0dc79fb72368d824ff6198cd1166a405bc5a7fc111d3a8",
		ContainerName: "badOne",
	})
	mockSender.On("ServiceCheck", "docker.exit", servicecheck.ServiceCheckCritical, "",
		[]string{"exit_code:1"}, "Container badOne exited with 1")

	dockerCheck.reportExitCodes(events, mockSender)
	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "ServiceCheck", 3)

	// Custom ok exit codes
	dockerCheck = &DockerCheck{
		instance: &DockerConfig{
			OkExitCodes: []int{0},
		},
		tagger: fakeTagger,
	}

	dockerCheck.setOkExitCodes()

	// Reset event array
	events = events[0:0]

	// Valid exit 0 event
	events = append(events, &docker.ContainerEvent{
		Action:        "die",
		Attributes:    map[string]string{"exitCode": "0"},
		ContainerID:   "fcc487ac70446287ae0dc79fb72368d824ff6198cd1166a405bc5a7fc111d3a8",
		ContainerName: "goodOne",
	})
	mockSender.On("ServiceCheck", "docker.exit", servicecheck.ServiceCheckOK, "",
		[]string{"exit_code:0"}, "Container goodOne exited with 0")

	// Valid exit 143 event
	events = append(events, &docker.ContainerEvent{
		Action:        "die",
		Attributes:    map[string]string{"exitCode": "143"},
		ContainerID:   "fcc487ac70446287ae0dc79fb72368d824ff6198cd1166a405bc5a7fc111d3a8",
		ContainerName: "badOne",
	})
	mockSender.On("ServiceCheck", "docker.exit", servicecheck.ServiceCheckCritical, "",
		[]string{"exit_code:143"}, "Container badOne exited with 143")

	dockerCheck.reportExitCodes(events, mockSender)
	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "ServiceCheck", 5)
}

func TestAggregateEvents(t *testing.T) {
	fakeTagger := taggerimpl.SetupFakeTagger(t)

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
					countByAction: map[events.Action]int{
						"unfiltered_action": 1,
					},
					alertType: event.AlertTypeInfo,
					tagger:    fakeTagger,
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
					countByAction: map[events.Action]int{
						"unfiltered_action": 2,
						"other_action":      1,
					},
					alertType: event.AlertTypeInfo,
					tagger:    fakeTagger,
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
					countByAction: map[events.Action]int{
						"unfiltered_action": 2,
						"other_action":      1,
					},
					alertType: event.AlertTypeInfo,
					tagger:    fakeTagger,
				},
				"other_image": {
					imageName: "other_image",
					countByAction: map[events.Action]int{
						"other_action": 1,
					},
					alertType: event.AlertTypeInfo,
					tagger:    fakeTagger,
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			transformer := newBundledTransformer("test-host", tc.filteredActions, fakeTagger).(*bundledTransformer)
			bundles := transformer.aggregateEvents(tc.events)
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
