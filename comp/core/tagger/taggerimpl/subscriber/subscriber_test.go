// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package subscriber

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

const (
	entityID = "foo://bar"
)

func TestSubscriber(t *testing.T) {
	events := map[string]types.EntityEvent{
		"added": {
			EventType: types.EventTypeAdded,
			Entity: types.Entity{
				ID: entityID,
			},
		},
		"modified": {
			EventType: types.EventTypeModified,
		},
		"deleted": {
			EventType: types.EventTypeDeleted,
		},
	}

	s := NewSubscriber()

	prevCh := s.Subscribe(types.LowCardinality, nil)

	s.Notify([]types.EntityEvent{
		events["added"],
		events["modified"],
	})

	newCh := s.Subscribe(types.LowCardinality, []types.EntityEvent{
		events["added"],
	})

	s.Notify([]types.EntityEvent{
		events["modified"],
		events["deleted"],
	})

	s.Unsubscribe(prevCh)
	s.Unsubscribe(newCh)

	expectedPrevChEvents := []types.EntityEvent{
		events["added"],
		events["modified"],
		events["modified"],
		events["deleted"],
	}
	expectedNewChEvents := []types.EntityEvent{
		events["added"],
		events["modified"],
		events["deleted"],
	}

	prevChEvents := []types.EntityEvent{}
	for e := range prevCh {
		prevChEvents = append(prevChEvents, e...)
	}

	newChEvents := []types.EntityEvent{}
	for e := range newCh {
		newChEvents = append(newChEvents, e...)
	}

	assert.Equal(t, expectedPrevChEvents, prevChEvents)
	assert.Equal(t, expectedNewChEvents, newChEvents)
}
