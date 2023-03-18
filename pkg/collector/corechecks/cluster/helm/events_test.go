// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package helm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	coreMetrics "github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestAddEventForNewRelease(t *testing.T) {
	events := eventsManager{}

	rel := release{
		Name: "my_datadog",
		Info: &info{
			Status: "deployed",
		},
		Chart: &chart{
			Metadata: &metadata{
				Name:       "datadog",
				Version:    "2.30.5",
				AppVersion: "7",
			},
		},
		Version:   1,
		Namespace: "default",
	}

	events.addEventForNewRelease(&rel, testTags())

	// Test new release with revision 1.
	storedEvent := events.events[0]
	storedEvent.Ts = 0 // Can't control it, so don't check.
	assert.Equal(
		t,
		coreMetrics.Event{
			Title:          "Event on Helm release",
			Text:           "New Helm release \"my_datadog\" has been deployed in \"default\" namespace. Its status is \"deployed\".",
			Ts:             0,
			Priority:       coreMetrics.EventPriorityNormal,
			SourceTypeName: "helm",
			EventType:      "helm",
			AggregationKey: "helm_release:default/my_datadog",
			Tags:           testTags(),
			AlertType:      coreMetrics.EventAlertTypeInfo,
		},
		storedEvent,
	)

	// Test release with upgraded revision (Notice that the text of the events is different).
	rel.Version = 2
	events.addEventForNewRelease(&rel, testTags())
	storedEvent = events.events[1]
	storedEvent.Ts = 0 // Can't control it, so don't check.
	assert.Equal(
		t,
		coreMetrics.Event{
			Title:          "Event on Helm release",
			Text:           "Helm release \"my_datadog\" in \"default\" namespace upgraded to revision 2. Its status is \"deployed\".",
			Ts:             0,
			Priority:       coreMetrics.EventPriorityNormal,
			SourceTypeName: "helm",
			EventType:      "helm",
			AggregationKey: "helm_release:default/my_datadog",
			Tags:           testTags(),
			AlertType:      coreMetrics.EventAlertTypeInfo,
		},
		storedEvent,
	)
}

func TestAddEventForDeletedRelease(t *testing.T) {
	events := eventsManager{}

	rel := release{
		Name: "my_datadog",
		Info: &info{
			Status: "deployed",
		},
		Chart: &chart{
			Metadata: &metadata{
				Name:       "datadog",
				Version:    "2.30.5",
				AppVersion: "7",
			},
		},
		Version:   1,
		Namespace: "default",
	}

	events.addEventForDeletedRelease(&rel, testTags())

	storedEvent := events.events[0]
	storedEvent.Ts = 0 // Can't control it, so don't check.
	assert.Equal(
		t,
		coreMetrics.Event{
			Title:          "Event on Helm release",
			Text:           "Helm release \"my_datadog\" in \"default\" namespace has been deleted.",
			Ts:             0,
			Priority:       coreMetrics.EventPriorityNormal,
			SourceTypeName: "helm",
			EventType:      "helm",
			AggregationKey: "helm_release:default/my_datadog",
			Tags:           testTags(),
			AlertType:      coreMetrics.EventAlertTypeInfo,
		},
		storedEvent,
	)
}

func TestAddEventForUpdatedRelease(t *testing.T) {
	exampleRelease := release{
		Name: "my_datadog",
		Info: &info{
			Status: "deployed",
		},
		Chart: &chart{
			Metadata: &metadata{
				Name:       "datadog",
				Version:    "2.30.5",
				AppVersion: "7",
			},
		},
		Version:   1,
		Namespace: "default",
	}

	exampleReleaseWithFailedStatus := release{
		Name: "my_datadog",
		Info: &info{
			Status: "failed",
		},
		Chart: &chart{
			Metadata: &metadata{
				Name:       "datadog",
				Version:    "2.30.5",
				AppVersion: "7",
			},
		},
		Version:   1,
		Namespace: "default",
	}

	exampleReleaseWithNonFailedStatus := release{
		Name: "my_datadog",
		Info: &info{
			Status: "uninstalling",
		},
		Chart: &chart{
			Metadata: &metadata{
				Name:       "datadog",
				Version:    "2.30.5",
				AppVersion: "7",
			},
		},
		Version:   1,
		Namespace: "default",
	}

	releaseWithoutInfo := release{
		Name: "my_datadog",
		Info: nil,
		Chart: &chart{
			Metadata: &metadata{
				Name:       "datadog",
				Version:    "2.30.5",
				AppVersion: "7",
			},
		},
		Version:   1,
		Namespace: "default",
	}

	tests := []struct {
		name           string
		oldRelease     *release
		updatedRelease *release
		expectedEvent  *coreMetrics.Event
	}{
		{
			name:           "The status changed to \"failed\"",
			oldRelease:     &exampleRelease,
			updatedRelease: &exampleReleaseWithFailedStatus,
			expectedEvent: &coreMetrics.Event{
				Title:          "Event on Helm release",
				Text:           "Helm release \"my_datadog\" (revision 1) in \"default\" namespace changed its status from \"deployed\" to \"failed\".",
				Ts:             0,
				Priority:       coreMetrics.EventPriorityNormal,
				SourceTypeName: "helm",
				EventType:      "helm",
				AggregationKey: "helm_release:default/my_datadog",
				Tags:           testTags(),
				AlertType:      coreMetrics.EventAlertTypeError, // Because the new status is "failed"
			},
		},
		{
			name:           "The status changed and it is not \"failed\"",
			oldRelease:     &exampleRelease,
			updatedRelease: &exampleReleaseWithNonFailedStatus,
			expectedEvent: &coreMetrics.Event{
				Title:          "Event on Helm release",
				Text:           "Helm release \"my_datadog\" (revision 1) in \"default\" namespace changed its status from \"deployed\" to \"uninstalling\".",
				Ts:             0,
				Priority:       coreMetrics.EventPriorityNormal,
				SourceTypeName: "helm",
				EventType:      "helm",
				AggregationKey: "helm_release:default/my_datadog",
				Tags:           testTags(),
				AlertType:      coreMetrics.EventAlertTypeInfo, // Because the new status is not "failed"
			},
		},
		{
			name:           "The status didn't change",
			oldRelease:     &exampleRelease,
			updatedRelease: &exampleRelease,
			expectedEvent:  nil,
		},
		{
			name:           "There's no info in the old release",
			oldRelease:     &releaseWithoutInfo,
			updatedRelease: &exampleRelease,
			expectedEvent:  nil,
		},
		{
			name:           "There's no info in the updated release",
			oldRelease:     &exampleRelease,
			updatedRelease: &releaseWithoutInfo,
			expectedEvent:  nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			events := eventsManager{}

			events.addEventForUpdatedRelease(test.oldRelease, test.updatedRelease, testTags())

			if test.expectedEvent == nil {
				assert.Empty(t, events.events)
			} else {
				storedEvent := events.events[0]
				storedEvent.Ts = 0 // Can't control it, so don't check.
				assert.Equal(t, test.expectedEvent, &storedEvent)
			}
		})
	}
}

func TestSendEvents(t *testing.T) {
	events := eventsManager{}

	rel := release{
		Name: "my_datadog",
		Info: &info{
			Status: "deployed",
		},
		Chart: &chart{
			Metadata: &metadata{
				Name:       "datadog",
				Version:    "2.30.5",
				AppVersion: "7",
			},
		},
		Version:   1,
		Namespace: "default",
	}

	events.addEventForNewRelease(&rel, testTags())

	sender := mocksender.NewMockSender("1")
	sender.SetupAcceptAll()
	events.sendEvents(sender)

	sender.AssertEvent(
		t,
		coreMetrics.Event{
			Title:          "Event on Helm release",
			Text:           "New Helm release \"my_datadog\" has been deployed in \"default\" namespace. Its status is \"deployed\".",
			Ts:             time.Now().Unix(),
			Priority:       coreMetrics.EventPriorityNormal,
			SourceTypeName: "helm",
			EventType:      "helm",
			AggregationKey: "helm_release:default/my_datadog",
			Tags:           testTags(),
			AlertType:      coreMetrics.EventAlertTypeInfo,
		},
		10*time.Second,
	)
}

func testTags() []string {
	return []string{"helm_release:my_datadog"}
}
