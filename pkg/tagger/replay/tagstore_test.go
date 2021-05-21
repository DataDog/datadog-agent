package replay

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/tagger/types"
)

const (
	entityID        = "foo://bar"
	anotherEntityID = "foo://quux"
)

func TestStore(t *testing.T) {
	store := newTagStore()
	e := types.Entity{
		ID:                 entityID,
		LowCardinalityTags: []string{"foo"},
	}

	store.addEntity(entityID, e)

	entity, ok := store.getEntity(entityID)
	assert.True(t, ok)
	assert.NotNil(t, entity)
	assert.Equal(t, []string{"foo"}, entity.LowCardinalityTags)
	assert.Equal(t, []string(nil), entity.OrchestratorCardinalityTags)
	assert.Equal(t, []string(nil), entity.HighCardinalityTags)
	assert.Equal(t, []string(nil), entity.StandardTags)

	// replace entity
	e = types.Entity{
		ID:                          entityID,
		LowCardinalityTags:          []string{"foo", "bar"},
		OrchestratorCardinalityTags: []string{"baz"},
	}
	store.addEntity(entityID, e)

	e = types.Entity{
		ID:                  anotherEntityID,
		LowCardinalityTags:  []string{"bar"},
		HighCardinalityTags: []string{"baz"},
	}
	store.addEntity(anotherEntityID, e)

	entity, ok = store.getEntity(entityID)
	assert.True(t, ok)
	assert.NotNil(t, entity)
	assert.Equal(t, []string{"foo", "bar"}, entity.LowCardinalityTags)
	assert.Equal(t, []string{"baz"}, entity.OrchestratorCardinalityTags)
	assert.Equal(t, []string(nil), entity.HighCardinalityTags)
	assert.Equal(t, []string(nil), entity.StandardTags)

	entity, ok = store.getEntity(anotherEntityID)
	assert.True(t, ok)
	assert.NotNil(t, entity)
	assert.Equal(t, []string{"bar"}, entity.LowCardinalityTags)
	assert.Equal(t, []string{"baz"}, entity.HighCardinalityTags)
	assert.Equal(t, []string(nil), entity.OrchestratorCardinalityTags)
	assert.Equal(t, []string(nil), entity.StandardTags)

	entities := store.listEntities()
	assert.Equal(t, 2, len(entities))

	store.reset()
	entities = store.listEntities()
	assert.Equal(t, 0, len(entities))

}
