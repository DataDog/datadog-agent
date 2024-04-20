// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package container

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

func TestStartError(t *testing.T) {
	workloadmetaStore := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModule(),
	))
	c := collector{
		store: workloadmetaStore,
	}

	err := c.Start(context.TODO(), workloadmetaStore)
	assert.Error(t, err)
}

func TestPull(t *testing.T) {
	workloadmetaStore := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModule(),
	))
	fakeNodeName := "fake-hostname"

	c := collector{
		store:    workloadmetaStore,
		nodeName: fakeNodeName,
	}

	err := c.Pull(context.TODO())
	assert.NoError(t, err)
	evs := workloadmetaStore.GetNotifiedEvents()
	assert.NotEmpty(t, evs)

	event0 := evs[0]

	assert.Equal(t, event0.Type, workloadmeta.EventTypeSet)
	assert.Equal(t, event0.Source, workloadmeta.SourceClusterOrchestrator)

	containerEntity, ok := event0.Entity.(*workloadmeta.Container)
	assert.True(t, ok)
	assert.Equal(t, containerEntity.ID, fakeNodeName)
}
