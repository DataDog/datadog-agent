// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package container

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestStartError(t *testing.T) {
	workloadmetaStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmetafxmock.MockModuleV2(),
	))
	c := collector{
		store: workloadmetaStore,
	}

	err := c.Start(context.TODO(), workloadmetaStore)
	assert.Error(t, err)
}

func TestPull(t *testing.T) {
	workloadmetaStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmetafxmock.MockModuleV2(),
	))
	fakeNodeName := "fake-hostname"

	c := collector{
		store:    workloadmetaStore,
		nodeName: fakeNodeName,
	}

	err := c.Pull(context.TODO())
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		container, err := workloadmetaStore.GetContainer(fakeNodeName)
		if err != nil {
			return false
		}
		return container.Runtime == workloadmeta.ContainerRuntimeGarden
	}, 10*time.Second, 50*time.Millisecond)
}
