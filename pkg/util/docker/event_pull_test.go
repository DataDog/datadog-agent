// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker && cel

package docker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/docker/docker/api/types/events"
	"github.com/stretchr/testify/assert"

	workloadfilterfxmock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestProcessContainerEvent(t *testing.T) {
	assert := assert.New(t)

	// Dummy timestamp for events
	timestamp := time.Now().Truncate(10 * time.Millisecond)

	configYaml := `
cel_workload_exclude:
  - products:
      - global
    rules:
      containers:
       - container.name == 'excluded_cel_name'
container_exclude: name:excluded_name image:excluded_image
`

	// Container filter
	configmock.NewFromYAML(t, configYaml)
	mockFilterStore := workloadfilterfxmock.SetupMockFilter(t)
	filter := mockFilterStore.GetContainerSharedMetricFilters()

	dockerUtil := &DockerUtil{
		cfg: &Config{},
	}

	for nb, tc := range []struct {
		source events.Message
		event  *ContainerEvent
		err    error
	}{
		{
			// Ignore empty events
			source: events.Message{},
			event:  nil,
			err:    nil,
		},
		{
			// Ignore non-container events
			source: events.Message{
				Type: "notcontainer",
			},
			event: nil,
			err:   nil,
		},
		{
			// Ignore prune events
			source: events.Message{
				Type:   "container",
				Action: "prune",
			},
			event: nil,
			err:   nil,
		},
		{
			// Error if container name not found
			source: events.Message{
				Type: "container",
				Actor: events.Actor{
					ID: "test_id",
				},
			},
			event: nil,
			err:   errors.New("missing container name in event"),
		},
		{
			// Error if image not found
			source: events.Message{
				Type: "container",
				Actor: events.Actor{
					ID: "test_id",
					Attributes: map[string]string{
						"name": "test_name",
					},
				},
			},
			event: nil,
			err:   errors.New("missing image name in event"),
		},
		{
			// Nominal case
			source: events.Message{
				Type: "container",
				Actor: events.Actor{
					ID: "test_id",
					Attributes: map[string]string{
						"name":      "test_name",
						"image":     "test_image",
						"extra_key": "value",
					},
				},
				Action:   "test_action",
				Time:     timestamp.Unix(),
				TimeNano: timestamp.UnixNano(),
			},
			event: &ContainerEvent{
				ContainerID:   "test_id",
				ContainerName: "test_name",
				ImageName:     "test_image",
				Action:        "test_action",
				Timestamp:     timestamp,
				Attributes: map[string]string{
					"name":      "test_name",
					"image":     "test_image",
					"extra_key": "value",
				},
			},
			err: nil,
		},
		{
			// Ignore excluded container name via cel
			source: events.Message{
				Type: "container",
				Actor: events.Actor{
					ID: "test_id",
					Attributes: map[string]string{
						"name":  "excluded_cel_name",
						"image": "test_image",
					},
				},
			},
			event: nil,
			err:   nil,
		},
		{
			// Ignore excluded container name
			source: events.Message{
				Type: "container",
				Actor: events.Actor{
					ID: "test_id",
					Attributes: map[string]string{
						"name":  "excluded_name",
						"image": "test_image",
					},
				},
			},
			event: nil,
			err:   nil,
		},
		{
			// Ignore excluded image name
			source: events.Message{
				Type: "container",
				Actor: events.Actor{
					ID: "test_id",
					Attributes: map[string]string{
						"name":  "test_name",
						"image": "excluded_image",
					},
				},
			},
			event: nil,
			err:   nil,
		},
		{
			// Fix bad action
			source: events.Message{
				Type: "container",
				Actor: events.Actor{
					ID: "test_id",
					Attributes: map[string]string{
						"name":      "test_name",
						"image":     "test_image",
						"extra_key": "value",
					},
				},
				Action:   "exec_start: /bin/sh -c true",
				Time:     timestamp.Unix(),
				TimeNano: timestamp.UnixNano(),
			},
			event: &ContainerEvent{
				ContainerID:   "test_id",
				ContainerName: "test_name",
				ImageName:     "test_image",
				Action:        "exec_start",
				Timestamp:     timestamp,
				Attributes: map[string]string{
					"name":      "test_name",
					"image":     "test_image",
					"extra_key": "value",
				},
			},
			err: nil,
		},
	} {
		ctx := context.Background()

		t.Logf("test case %d", nb)
		event, err := dockerUtil.processContainerEvent(ctx, tc.source, filter)
		assert.Equal(tc.event, event)

		if tc.err == nil {
			assert.Nil(err)
		} else {
			assert.NotNil(err)
			assert.Contains(err.Error(), tc.err.Error())
		}
	}
}

func TestProcessImageEvent(t *testing.T) {
	timestamp := time.Now().Truncate(10 * time.Millisecond)

	tests := []struct {
		name               string
		message            events.Message
		expectedImageEvent *ImageEvent
	}{
		{
			name:               "empty event",
			message:            events.Message{},
			expectedImageEvent: nil,
		},
		{
			name: "non-image event",
			message: events.Message{
				Type: events.ContainerEventType,
			},
			expectedImageEvent: nil,
		},
		{
			name: "standard case",
			message: events.Message{
				Type:   events.ImageEventType,
				Action: events.ActionPull,
				Actor: events.Actor{
					ID: "agent:latest",
				},
				Time:     timestamp.Unix(),
				TimeNano: timestamp.UnixNano(),
			},
			expectedImageEvent: &ImageEvent{
				ImageID:   "agent:latest",
				Action:    events.ActionPull,
				Timestamp: timestamp,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dockerUtil := &DockerUtil{}
			imageEvent := dockerUtil.processImageEvent(test.message)
			assert.Equal(t, test.expectedImageEvent, imageEvent)
		})
	}
}
