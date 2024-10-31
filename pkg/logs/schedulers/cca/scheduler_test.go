// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package cca

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	logsConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func setup(t *testing.T) (scheduler *Scheduler, ac autodiscovery.Component, spy *schedulers.MockSourceManager) {
	ac = fxutil.Test[autodiscovery.Mock](t,
		fx.Supply(autodiscoveryimpl.MockParams{}),
		secretsimpl.MockModule(),
		autodiscoveryimpl.MockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		core.MockBundle(),
		fx.Supply(tagger.NewFakeTaggerParams()),
		fx.Provide(taggerimpl.NewMock),
	)
	scheduler = New(ac).(*Scheduler)
	spy = &schedulers.MockSourceManager{}
	return
}

func TestNothingWhenNoConfig(t *testing.T) {
	scheduler, _, spy := setup(t)
	config := configmock.New(t)
	config.SetWithoutSource("logs_config.container_collect_all", false)

	scheduler.Start(spy)

	require.Equal(t, 0, len(spy.Events))
}

func TestAfterACStarts(t *testing.T) {
	scheduler, ac, spy := setup(t)
	config := configmock.New(t)
	config.SetWithoutSource("logs_config.container_collect_all", true)

	scheduler.Start(spy)

	// nothing added yet
	require.Equal(t, 0, len(spy.Events))

	// Fake autoconfig running..
	ac.ForceRanOnceFlag()

	// wait for the source to be added
	<-scheduler.added

	source := spy.Events[0].Source
	assert.Equal(t, "container_collect_all", source.Name)
	assert.Equal(t, logsConfig.DockerType, source.Config.Type)
	assert.Equal(t, "docker", source.Config.Source)
	assert.Equal(t, "docker", source.Config.Service)
}
