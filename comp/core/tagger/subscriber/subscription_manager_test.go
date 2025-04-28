// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package subscriber

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taggerTelemetry "github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestSubscriptionManager(t *testing.T) {

	entityID := types.NewEntityID(types.ContainerID, "bar")

	events := map[string]types.EntityEvent{
		"added": {
			EventType: types.EventTypeAdded,
			Entity: types.Entity{
				ID:                          entityID,
				HighCardinalityTags:         []string{"t1:v1", "t2:v2", "t3:v3"},
				OrchestratorCardinalityTags: []string{"t1:v1", "t2:v2"},
				LowCardinalityTags:          []string{"t1:v1"},
				StandardTags:                []string{"s1:v1"},
			},
		},
		"modified": {
			EventType: types.EventTypeModified,
			Entity: types.Entity{
				ID:                          entityID,
				HighCardinalityTags:         []string{"t1:v1", "t2:v2", "t3:v3", "t4:v4"},
				OrchestratorCardinalityTags: []string{"t1:v1", "t2:v2"},
				LowCardinalityTags:          []string{"t1:v1"},
				StandardTags:                []string{"s1:v1"},
			},
		},
		"deleted": {
			EventType: types.EventTypeDeleted,
			Entity: types.Entity{
				ID: entityID,
			},
		},
		"added-with-no-id": {
			EventType: types.EventTypeAdded,
		},
		"added-with-unmatched-prefix": {
			EventType: types.EventTypeAdded,
			Entity: types.Entity{
				// KubernetesDeployment prefix doesn't match any of the subscription filters used below
				ID: types.NewEntityID(types.KubernetesDeployment, "goo"),
			},
		},
	}
	tel := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
	telemetryStore := taggerTelemetry.NewStore(tel)
	sm := NewSubscriptionManager(telemetryStore)

	// Low Cardinality Subscriber
	lowCardSubID := "low-card-sub"
	lowCardSubscription, err := sm.Subscribe(lowCardSubID, types.NewFilterBuilder().Include(types.ContainerID).Build(types.LowCardinality), nil)
	require.NoError(t, err)

	sm.Notify([]types.EntityEvent{
		events["added"],
		events["modified"],
		events["deleted"],
		events["added-with-no-id"],
		events["added-with-unmatched-prefix"],
	})

	lowCardSubscription.Unsubscribe()

	// Orchestrator Cardinality Subscriber
	orchCardSubID := "orch-card-sub"
	orchCardSubscription, err := sm.Subscribe(orchCardSubID, types.NewFilterBuilder().Include(types.ContainerID).Build(types.OrchestratorCardinality), nil)
	require.NoError(t, err)

	sm.Notify([]types.EntityEvent{
		events["added"],
		events["modified"],
		events["deleted"],
		events["added-with-no-id"],
		events["added-with-unmatched-prefix"],
	})

	orchCardSubscription.Unsubscribe()

	// High Cardinality Subscriber
	highCardSubID := "high-card-sub"
	highCardSubscription, err := sm.Subscribe(highCardSubID, types.NewFilterBuilder().Include(types.ContainerID).Build(types.HighCardinality), []types.EntityEvent{
		events["added"],
	})
	require.NoError(t, err)

	sm.Notify([]types.EntityEvent{
		events["modified"],
		events["deleted"],
		events["added-with-no-id"],
		events["added-with-unmatched-prefix"],
	})

	highCardSubscription.Unsubscribe()

	// None Cardinality Subscriber
	noneCardSubID := "none-card-sub"
	noneCardSubscription, err := sm.Subscribe(noneCardSubID, types.NewFilterBuilder().Include(types.ContainerID).Build(types.NoneCardinality), nil)
	require.NoError(t, err)

	sm.Notify([]types.EntityEvent{
		events["added"],
		events["modified"],
		events["deleted"],
		events["added-with-no-id"],
		events["added-with-unmatched-prefix"],
	})

	noneCardSubscription.Unsubscribe()

	// Verify low cardinality subscriber received events
	assertReceivedEvents(t, lowCardSubscription.EventsChan(), []types.EntityEvent{
		{
			EventType: types.EventTypeAdded,
			Entity: types.Entity{
				ID:                 entityID,
				LowCardinalityTags: []string{"t1:v1"},
				StandardTags:       []string{"s1:v1"},
			},
		},
		{
			EventType: types.EventTypeModified,
			Entity: types.Entity{
				ID:                 entityID,
				LowCardinalityTags: []string{"t1:v1"},
				StandardTags:       []string{"s1:v1"},
			},
		},
		{
			EventType: types.EventTypeDeleted,
			Entity: types.Entity{
				ID: entityID,
			},
		},
	})

	// Verify orchestrator cardinality subscriber received events
	assertReceivedEvents(t, orchCardSubscription.EventsChan(), []types.EntityEvent{
		{
			EventType: types.EventTypeAdded,
			Entity: types.Entity{
				ID:                          entityID,
				OrchestratorCardinalityTags: []string{"t1:v1", "t2:v2"},
				LowCardinalityTags:          []string{"t1:v1"},
				StandardTags:                []string{"s1:v1"},
			},
		},
		{
			EventType: types.EventTypeModified,
			Entity: types.Entity{
				ID:                          entityID,
				OrchestratorCardinalityTags: []string{"t1:v1", "t2:v2"},
				LowCardinalityTags:          []string{"t1:v1"},
				StandardTags:                []string{"s1:v1"},
			},
		},
		{
			EventType: types.EventTypeDeleted,
			Entity: types.Entity{
				ID: entityID,
			},
		},
	})

	// Verify high cardinality subscriber received events
	assertReceivedEvents(t, highCardSubscription.EventsChan(), []types.EntityEvent{
		{
			EventType: types.EventTypeAdded,
			Entity: types.Entity{
				ID:                          entityID,
				HighCardinalityTags:         []string{"t1:v1", "t2:v2", "t3:v3"},
				OrchestratorCardinalityTags: []string{"t1:v1", "t2:v2"},
				LowCardinalityTags:          []string{"t1:v1"},
				StandardTags:                []string{"s1:v1"},
			},
		},
		{
			EventType: types.EventTypeModified,
			Entity: types.Entity{
				ID:                          entityID,
				HighCardinalityTags:         []string{"t1:v1", "t2:v2", "t3:v3", "t4:v4"},
				OrchestratorCardinalityTags: []string{"t1:v1", "t2:v2"},
				LowCardinalityTags:          []string{"t1:v1"},
				StandardTags:                []string{"s1:v1"},
			},
		},
		{
			EventType: types.EventTypeDeleted,
			Entity: types.Entity{
				ID: entityID,
			},
		},
	})

	// Verify none cardinality subscriber received events
	assertReceivedEvents(t, noneCardSubscription.EventsChan(), []types.EntityEvent{
		{
			EventType: types.EventTypeAdded,
			Entity: types.Entity{
				ID: entityID,
			},
		},
		{
			EventType: types.EventTypeModified,
			Entity: types.Entity{
				ID: entityID,
			},
		},
		{
			EventType: types.EventTypeDeleted,
			Entity: types.Entity{
				ID: entityID,
			},
		},
	})
}

func assertReceivedEvents(t *testing.T, ch chan []types.EntityEvent, expectedEvents []types.EntityEvent) {
	receivedEvents := []types.EntityEvent{}

	for e := range ch {
		receivedEvents = append(receivedEvents, e...)
	}

	assert.ElementsMatch(t, receivedEvents, expectedEvents)
}

func TestSubscriptionManagerDuplicateSubscriberID(t *testing.T) {
	tel := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
	telemetryStore := taggerTelemetry.NewStore(tel)
	sm := NewSubscriptionManager(telemetryStore)

	// Low Cardinality Subscriber
	lowCardSubID := "low-card-sub"
	_, err := sm.Subscribe(lowCardSubID, types.NewFilterBuilder().Include(types.ContainerID).Build(types.LowCardinality), nil)
	require.NoError(t, err)

	_, err = sm.Subscribe(lowCardSubID, types.NewFilterBuilder().Include(types.ContainerID).Build(types.LowCardinality), nil)
	require.Error(t, err)
}
