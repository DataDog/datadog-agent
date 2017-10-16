// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package containers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

func TestReportExitCodes(t *testing.T) {
	dockerCheck := &DockerCheck{
		instance: &DockerConfig{},
	}
	mockSender := aggregator.NewMockSender(dockerCheck.ID())

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
