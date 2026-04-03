// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/listeners"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap/pidmapimpl"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	replaymock "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/fx-mock"
	serverdebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug/serverdebugimpl"
	filterlistimpl "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// depsWithoutFilterList is like depsWithoutServer but without the FilterList field,
// so we can inject a real filterlist implementation manually.
type depsWithoutFilterList struct {
	fx.In

	Config        configComponent.Component
	Log           log.Component
	Demultiplexer demultiplexer.FakeSamplerMock
	Replay        replay.Component
	PidMap        pidmap.Component
	Debug         serverdebug.Component
	WMeta         option.Option[workloadmeta.Component]
	Telemetry     telemetry.Component
	Hostname      hostnameinterface.Component
}

func TestWorkerFilterListInitializedFromLocalConfig(t *testing.T) {
	cfg := map[string]interface{}{
		"dogstatsd_port":    listeners.RandomPortName,
		"metric_filterlist": []string{"filtered.metric"},
	}

	deps := fxutil.Test[depsWithoutFilterList](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() configComponent.Component { return configComponent.NewMockWithOverrides(t, cfg) }),
		telemetryimpl.MockModule(),
		hostnameimpl.MockModule(),
		serverdebugimpl.MockModule(),
		replaymock.MockModule(),
		pidmapimpl.Module(),
		demultiplexerimpl.FakeSamplerMockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		metricscompression.MockModule(),
		logscompression.MockModule(),
	))

	fl := filterlistimpl.NewFilterList(deps.Log, deps.Config, deps.Telemetry)
	s := newServerCompat(deps.Config, deps.Log, deps.Hostname, deps.Replay, deps.Debug, false, deps.Demultiplexer, deps.WMeta, deps.PidMap, deps.Telemetry, fl)

	err := s.start(context.TODO())
	require.NoError(t, err)
	defer s.stop(context.TODO()) //nolint:errcheck

	require.NotEmpty(t, s.workers, "server should have workers")
	for i, worker := range s.workers {
		require.NotNil(t, worker.filterList, "worker %d filterList should not be nil", i)
		assert.True(t, worker.filterList.Test("filtered.metric"),
			"worker %d should filter 'filtered.metric' from local config", i)
		assert.False(t, worker.filterList.Test("unfiltered.metric"),
			"worker %d should not filter 'unfiltered.metric'", i)
	}
}
