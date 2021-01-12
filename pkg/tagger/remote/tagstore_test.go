package remote

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/tagger/types"
)

const (
	entityID        = "foo://bar"
	anotherEntityID = "foo://quux"
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

	store := newTagStore()

	for _, e := range events {
		store.processEvent(e)
	}

	entity, err := store.getEntity(entityID)
	if err != nil {
		t.Fatalf("got unexpected error: %s", err)
	}

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

	store := newTagStore()

	for _, e := range events {
		store.processEvent(e)
	}

	entity, err := store.getEntity(entityID)
	if err != nil {
		t.Fatalf("got unexpected error: %s", err)
	}

	assert.Nil(t, entity)

	entity, err = store.getEntity(anotherEntityID)
	if err != nil {
		t.Fatalf("got unexpected error: %s", err)
	}

	assert.NotNil(t, entity)
}
