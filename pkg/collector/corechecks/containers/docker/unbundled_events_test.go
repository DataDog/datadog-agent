// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

func TestUnbundledEventsTransform(t *testing.T) {
	hostname := "test-host"
	codeBlock := "```"
	eventBlock := "%%%"
	incomingEvents := []*docker.ContainerEvent{
		{
			ContainerID:   "5797e0079f48037cb194683a571ffe270bdf93dde9b4f5ec26371d98e727dec7",
			ContainerName: "squirtle",
			ImageName:     "pokemon/squirtle",
			Action:        events.ActionStart,
			Timestamp:     time.Now(),
			Attributes: map[string]string{
				"squirtle_attribute1": "value1",
				"squirtle_attribute2": "value2",
				"squirtle_attribute3": "value3",
			},
		},
		{
			ContainerID:   "5797e0079f48037cb194683a571ffe270bdf93dde9b4f5ec26371d98e727dec7",
			ContainerName: "squirtle",
			ImageName:     "pokemon/squirtle",
			Action:        events.ActionExecStart,
			Timestamp:     time.Now(),
			Attributes: map[string]string{
				"squirtle_attribute1": "value1",
				"squirtle_attribute2": "value2",
				"squirtle_attribute3": "value3",
			},
		},
		{
			ContainerID:   "5797e0079f48037cb194683a571ffe270bdf93dde9b4f5ec26371d98e727dec7",
			ContainerName: "squirtle",
			ImageName:     "pokemon/squirtle",
			Action:        events.ActionExecCreate,
			Timestamp:     time.Now(),
			Attributes: map[string]string{
				"squirtle_attribute1": "value1",
				"squirtle_attribute2": "value2",
				"squirtle_attribute3": "value3",
			},
		},
		{
			ContainerID:   "7579e0790f48037c1b49683a751ff2e70bdf93dde9b54fec26371d9e8727edc7",
			ContainerName: "azurill",
			ImageName:     "pokemon/azurill",
			Action:        events.ActionTop,
			Timestamp:     time.Now(),
			Attributes: map[string]string{
				"azurill_attribute1": "value1",
				"azurill_attribute2": "value2",
				"azurill_attribute3": "value3",
			},
		},
		{
			ContainerID:   "87b6a7785a962622fbdc2634ae69118913e3a4a7844f940a71c7db63532b265b",
			ContainerName: "bagon",
			ImageName:     "pokemon/bagon",
			Action:        events.ActionExecStart,
			Timestamp:     time.Now(),
			Attributes: map[string]string{
				"bagon_attribute1": "value1",
				"bagon_attribute2": "value2",
				"bagon_attribute3": "value3",
			},
		},
		{
			ContainerID:   "66b6a7785a962622fdbc2634a6e9191831e3aa48744f940a17cd7b36532b2b65",
			ContainerName: "bagon",
			ImageName:     "pokemon/bagon",
			Action:        events.ActionExecDie,
			Timestamp:     time.Now(),
			Attributes: map[string]string{
				"bagon_attribute1": "value1",
				"bagon_attribute2": "value2",
				"bagon_attribute3": "value3",
			},
		},
		{
			ContainerID:   "66b6a7785a962622fdbc2634a6e9191831e3aa48744f940a17cd7b36532b2b65",
			ContainerName: "bagon",
			ImageName:     "pokemon/bagon",
			Action:        events.ActionCopy,
			Timestamp:     time.Now(),
			Attributes: map[string]string{
				"bagon_attribute1": "value1",
				"bagon_attribute2": "value2",
				"bagon_attribute3": "value3",
			},
		},
		{
			ContainerID:   "26b6a7785a966222fdbc2364ae69191f13ea3a487449f40a17cd7b356322a6bb",
			ContainerName: "bagon",
			ImageName:     "pokemon/bagon",
			Action:        events.ActionDisable,
			Timestamp:     time.Now(),
			Attributes: map[string]string{
				"bagon_attribute1": "value1",
				"bagon_attribute2": "value2",
				"bagon_attribute3": "value3",
			},
		},
	}

	fakeTagger := taggerimpl.SetupFakeTagger(t)

	for _, ev := range incomingEvents {
		fakeTagger.SetTags(
			types.NewEntityID(types.ContainerID, ev.ContainerID),
			"docker",
			[]string{fmt.Sprintf("image_name:%s", ev.ImageName), fmt.Sprintf("container_name:%s", ev.ContainerName)},
			[]string{},
			[]string{},
			[]string{},
		)
	}

	tests := []struct {
		name                   string
		bundleUnspecifedEvents bool
		filteredEventTypes     []string
		collectedEventTypes    []string
		expectedEvents         []event.Event
	}{
		{
			name:                   "nothing will be unbundled since collectedEventTypes is empty",
			bundleUnspecifedEvents: false,
			filteredEventTypes:     []string{},
			collectedEventTypes:    []string{},
			expectedEvents:         nil,
		},
		{
			name:                   "bundle events with default filtered event types and without collected event types",
			bundleUnspecifedEvents: true,
			filteredEventTypes:     defaultFilteredEventTypes,
			collectedEventTypes:    []string{},
			expectedEvents: []event.Event{
				{
					Title: "pokemon/bagon 1 copy 1 disable on test-host",
					Text: fmt.Sprintf(`%s 
pokemon/bagon 1 copy 1 disable on test-host
%s
%s
%s
%s
 %s`, eventBlock, codeBlock, "COPY\tbagon", "DISABLE\tbagon", codeBlock, eventBlock),
					Priority:       "normal",
					Host:           hostname,
					AlertType:      "info",
					SourceTypeName: "docker",
					AggregationKey: "docker:pokemon/bagon",
					EventType:      "docker",
					Tags: []string{
						"image_name:pokemon/bagon",
						"container_name:bagon",
						"image_name:pokemon/bagon",
						"container_name:bagon",
					},
				},
				{
					Title: "pokemon/squirtle 1 start on test-host",
					Text: fmt.Sprintf(`%s 
pokemon/squirtle 1 start on test-host
%s
%s
%s
 %s`, eventBlock, codeBlock, "START\tsquirtle", codeBlock, eventBlock),
					Priority:       "normal",
					Host:           hostname,
					AlertType:      "info",
					EventType:      "docker",
					AggregationKey: "docker:pokemon/squirtle",
					SourceTypeName: "docker",
					Tags: []string{
						"image_name:pokemon/squirtle",
						"container_name:squirtle",
					},
				},
			},
		},
		{
			name:                   "unbundle and bundle events",
			bundleUnspecifedEvents: true,
			filteredEventTypes:     []string{"exec_start", "exec_die"},
			collectedEventTypes: []string{
				string(events.ActionCopy),
				string(events.ActionDisable),
				string(events.ActionStart),
			},
			expectedEvents: []event.Event{
				{
					Title: "pokemon/squirtle 1 exec_create on test-host",
					Text: fmt.Sprintf(`%s 
pokemon/squirtle 1 exec_create on test-host
%s
%s
%s
 %s`, eventBlock, codeBlock, "EXEC_CREATE\tsquirtle", codeBlock, eventBlock),
					Host:           hostname,
					Priority:       "normal",
					SourceTypeName: "docker",
					AggregationKey: "docker:pokemon/squirtle",
					AlertType:      "info",
					EventType:      "docker",
					Tags: []string{
						"image_name:pokemon/squirtle",
						"container_name:squirtle",
					},
				},
				{
					Title: "pokemon/azurill 1 top on test-host",
					Text: fmt.Sprintf(`%s 
pokemon/azurill 1 top on test-host
%s
%s
%s
 %s`, eventBlock, codeBlock, "TOP\tazurill", codeBlock, eventBlock),
					Host:           hostname,
					Priority:       "normal",
					SourceTypeName: "docker",
					AggregationKey: "docker:pokemon/azurill",
					AlertType:      "info",
					EventType:      "docker",
					Tags: []string{
						"image_name:pokemon/azurill",
						"container_name:azurill",
					},
				},
				{
					Title:          "Container 26b6a7785a966222fdbc2364ae69191f13ea3a487449f40a17cd7b356322a6bb: disable",
					Text:           `Container 26b6a7785a966222fdbc2364ae69191f13ea3a487449f40a17cd7b356322a6bb (running image "pokemon/bagon"): disable`,
					Host:           hostname,
					Priority:       "normal",
					AggregationKey: "docker:26b6a7785a966222fdbc2364ae69191f13ea3a487449f40a17cd7b356322a6bb",
					AlertType:      "info",
					SourceTypeName: "docker",
					Tags: []string{
						"image_name:pokemon/bagon",
						"container_name:bagon",
						"event_type:disable",
					},
				},
				{
					Title:          "Container 5797e0079f48037cb194683a571ffe270bdf93dde9b4f5ec26371d98e727dec7: start",
					Text:           `Container 5797e0079f48037cb194683a571ffe270bdf93dde9b4f5ec26371d98e727dec7 (running image "pokemon/squirtle"): start`,
					Host:           hostname,
					Priority:       "normal",
					AggregationKey: "docker:5797e0079f48037cb194683a571ffe270bdf93dde9b4f5ec26371d98e727dec7",
					AlertType:      "info",
					SourceTypeName: "docker",
					Tags: []string{
						"image_name:pokemon/squirtle",
						"container_name:squirtle",
						"event_type:start",
					},
				},
				{
					Title:          "Container 66b6a7785a962622fdbc2634a6e9191831e3aa48744f940a17cd7b36532b2b65: copy",
					Text:           `Container 66b6a7785a962622fdbc2634a6e9191831e3aa48744f940a17cd7b36532b2b65 (running image "pokemon/bagon"): copy`,
					Host:           hostname,
					Priority:       "normal",
					AggregationKey: "docker:66b6a7785a962622fdbc2634a6e9191831e3aa48744f940a17cd7b36532b2b65",
					AlertType:      "info",
					SourceTypeName: "docker",
					Tags: []string{
						"image_name:pokemon/bagon",
						"container_name:bagon",
						"event_type:copy",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var eventTransformer eventTransformer = noopEventTransformer{}
			if tt.bundleUnspecifedEvents {
				eventTransformer = newBundledTransformer(hostname, tt.filteredEventTypes, fakeTagger)
			}
			transformer := newUnbundledTransformer(
				hostname,
				tt.collectedEventTypes,
				eventTransformer,
				fakeTagger,
			)
			evs, errs := transformer.Transform(incomingEvents)
			require.Nil(t, errs)
			slices.SortFunc(evs, func(a, b event.Event) int {
				aSplitedTitle := strings.Split(a.Title, " ")
				bSplitedTitle := strings.Split(b.Title, " ")
				slices.Sort(aSplitedTitle)
				slices.Sort(bSplitedTitle)
				return cmp.Compare[string](
					strings.Join(aSplitedTitle, " "),
					strings.Join(bSplitedTitle, " "),
				)
			})
			require.Len(t, evs, len(tt.expectedEvents))
			require.Condition(t, func() (success bool) {
				for i, expected := range tt.expectedEvents {
					// Compare the semantics of the title and text to avoid character order issues.
					actualSplitedTitle := strings.Split(evs[i].Title, " ")
					expectedSplitedTitle := strings.Split(expected.Title, " ")
					slices.Sort(actualSplitedTitle)
					slices.Sort(expectedSplitedTitle)
					if !assert.Equalf(
						t,
						expectedSplitedTitle,
						actualSplitedTitle,
						"EXPECTED: %s\nACTUAL: %s",
						expected.Title,
						evs[i].Title,
					) {
						return false
					}
					actualSplitedText := strings.Split(evs[i].Text, " ")
					expectedSplitedText := strings.Split(expected.Text, " ")
					slices.Sort(actualSplitedText)
					slices.Sort(expectedSplitedText)
					if !assert.Equalf(
						t,
						expectedSplitedText,
						actualSplitedText,
						"EXPECTED: %s\nACTUAL: %s",
						expected.Text,
						evs[i].Text,
					) {
						return false
					}
					// Compare the rest of the fields.
					if !assert.Equal(t, expected.Host, evs[i].Host, "Host is not equal") {
						return false
					}
					if !assert.Equal(t, expected.AggregationKey, evs[i].AggregationKey, "AggregationKey is not equal") {
						return false
					}
					if !assert.Equal(t, expected.AlertType, evs[i].AlertType, "AlertType is not equal") {
						return false
					}
					if !assert.Equal(t, expected.SourceTypeName, evs[i].SourceTypeName, "SourceTypeName is not equal") {
						return false
					}
					if !assert.Equal(t, expected.Tags, evs[i].Tags, "Tags are not equal") {
						return false
					}
				}
				return true
			})
		})
	}
}
