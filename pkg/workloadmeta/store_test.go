// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/errors"
)

func TestHandleEvents(t *testing.T) {
	s := NewStore()

	container := Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "container_id://foobar",
		},
		EntityMeta: EntityMeta{
			Name: "foobar",
		},
		Runtime: "docker",
	}

	s.handleEvents([]Event{
		{
			Type:   EventTypeSet,
			Source: fooSource,
			Entity: container,
		},
	})

	gotContainer, err := s.GetContainer(container.ID)
	if err != nil {
		t.Errorf("expected to find container %q, not found", container.ID)
	}

	if !reflect.DeepEqual(container, gotContainer) {
		t.Errorf("expected container %q to match the one in the store", container.ID)
	}

	s.handleEvents([]Event{
		{
			Type:   EventTypeUnset,
			Source: fooSource,
			Entity: container,
		},
	})

	_, err = s.GetContainer(container.ID)
	if err == nil || !errors.IsNotFound(err) {
		t.Errorf("expected container %q to be absent. found or had errors. err: %q", container.ID, err)
	}
}
