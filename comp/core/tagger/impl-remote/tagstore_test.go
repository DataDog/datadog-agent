// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taggerTelemetry "github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	mocktelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

var (
	entityID        types.EntityID = types.NewEntityID(types.ContainerID, "bar")
	anotherEntityID types.EntityID = types.NewEntityID(types.ContainerID, "quux")
)

func TestProcessEvent_AddAndModify(t *testing.T) {
	events := []types.EntityEvent{
		{
			EventType: types.EventTypeAdded,
			Entity: types.Entity{
				ID:                 entityID,
				LowCardinalityTags: []string{"foo"},
			},
		},
		{
			EventType: types.EventTypeModified,
			Entity: types.Entity{
				ID:                          entityID,
				LowCardinalityTags:          []string{"foo", "bar"},
				OrchestratorCardinalityTags: []string{"baz"},
			},
		},
		{
			EventType: types.EventTypeAdded,
			Entity: types.Entity{
				ID:                 anotherEntityID,
				LowCardinalityTags: []string{"quux"},
			},
		},
	}
	tel := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	telemetryStore := taggerTelemetry.NewStore(tel)
	store := newTagStore(telemetryStore)
	store.processEvents(events, false)

	entity := store.getEntity(entityID)

	assert.Equal(t, []string{"foo", "bar"}, entity.LowCardinalityTags)
	assert.Equal(t, []string{"baz"}, entity.OrchestratorCardinalityTags)
	assert.Equal(t, []string(nil), entity.HighCardinalityTags)
	assert.Equal(t, []string(nil), entity.StandardTags)
}

func TestProcessEvent_AddAndDelete(t *testing.T) {
	events := []types.EntityEvent{
		{
			EventType: types.EventTypeAdded,
			Entity: types.Entity{
				ID:                 entityID,
				LowCardinalityTags: []string{"foo"},
			},
		},
		{
			EventType: types.EventTypeAdded,
			Entity: types.Entity{
				ID:                 anotherEntityID,
				LowCardinalityTags: []string{"quux"},
			},
		},
		{
			EventType: types.EventTypeDeleted,
			Entity: types.Entity{
				ID: entityID,
			},
		},
	}

	tel := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	telemetryStore := taggerTelemetry.NewStore(tel)
	store := newTagStore(telemetryStore)
	store.processEvents(events, false)

	entity := store.getEntity(entityID)

	assert.Nil(t, entity)

	entity = store.getEntity(anotherEntityID)

	assert.NotNil(t, entity)
}

func TestProcessEvent_Replace(t *testing.T) {
	tel := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	telemetryStore := taggerTelemetry.NewStore(tel)
	store := newTagStore(telemetryStore)

	store.processEvents([]types.EntityEvent{
		{
			EventType: types.EventTypeAdded,
			Entity: types.Entity{
				ID:                 entityID,
				LowCardinalityTags: []string{"foo"},
			},
		},
	}, false)

	store.processEvents([]types.EntityEvent{
		{
			EventType: types.EventTypeAdded,
			Entity: types.Entity{
				ID:                 anotherEntityID,
				LowCardinalityTags: []string{"foo"},
			},
		},
	}, true)

	entity := store.getEntity(entityID)

	assert.Nil(t, entity)

	entity = store.getEntity(anotherEntityID)

	assert.NotNil(t, entity)
}

func TestSubscribe(t *testing.T) {
	tel := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	telemetryStore := taggerTelemetry.NewStore(tel)
	store := newTagStore(telemetryStore)

	err := store.processEvents([]types.EntityEvent{
		{
			EventType: types.EventTypeAdded,
			Entity: types.Entity{
				ID:                          entityID,
				OrchestratorCardinalityTags: []string{"image_name:nginx"},
			},
		},
	}, false)
	require.NoError(t, err)

	filter := types.NewFilterBuilder().Include(types.ContainerID).Build(types.OrchestratorCardinality)
	sub, err := store.subscribe("test-sub", filter)
	require.NoError(t, err)
	defer sub.Unsubscribe()

	initialBatch := <-sub.EventsChan()
	require.Len(t, initialBatch, 1)
	assert.Equal(t, types.EventTypeAdded, initialBatch[0].EventType)
	assert.Equal(t, entityID, initialBatch[0].Entity.ID)
	assert.Equal(t, []string{"image_name:nginx"}, initialBatch[0].Entity.OrchestratorCardinalityTags)

	err = store.processEvents([]types.EntityEvent{
		{
			EventType: types.EventTypeModified,
			Entity: types.Entity{
				ID:                          entityID,
				OrchestratorCardinalityTags: []string{"image_name:nginx", "image_tag:1.27"},
			},
		},
	}, false)
	require.NoError(t, err)

	updateBatch := <-sub.EventsChan()
	require.Len(t, updateBatch, 1)
	assert.Equal(t, types.EventTypeModified, updateBatch[0].EventType)
	assert.Equal(t, []string{"image_name:nginx", "image_tag:1.27"}, updateBatch[0].Entity.OrchestratorCardinalityTags)
}
