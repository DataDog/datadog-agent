// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package container

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/stretchr/testify/assert"
)

type fakeWorkloadmetaStore struct {
	workloadmeta.Store
	notifiedEvents []workloadmeta.CollectorEvent
}

func (store *fakeWorkloadmetaStore) Notify(events []workloadmeta.CollectorEvent) {
	store.notifiedEvents = append(store.notifiedEvents, events...)
}

func TestStartError(t *testing.T) {
	workloadmetaStore := fakeWorkloadmetaStore{}
	c := collector{
		store: &workloadmetaStore,
	}

	err := c.Start(context.TODO(), &workloadmetaStore)
	assert.Error(t, err)
}

func TestPull(t *testing.T) {
	workloadmetaStore := fakeWorkloadmetaStore{}
	fakeNodeName := "fake-hostname"

	c := collector{
		store:    &workloadmetaStore,
		nodeName: fakeNodeName,
	}

	err := c.Pull(context.TODO())
	assert.NoError(t, err)
	assert.NotEmpty(t, workloadmetaStore.notifiedEvents)

	event0 := workloadmetaStore.notifiedEvents[0]

	assert.Equal(t, event0.Type, workloadmeta.EventTypeSet)
	assert.Equal(t, event0.Source, workloadmeta.SourceClusterOrchestrator)

	containerEntity, ok := event0.Entity.(*workloadmeta.Container)
	assert.True(t, ok)
	assert.Equal(t, containerEntity.ID, fakeNodeName)
}
