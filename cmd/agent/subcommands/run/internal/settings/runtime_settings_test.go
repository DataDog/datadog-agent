// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/comp/dogstatsd"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	serverdebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type testDeps struct {
	fx.In
	Config        config.Component
	Server        server.Component
	Debug         serverdebug.Component
	Demultiplexer demultiplexer.Mock
}

func TestDogstatsdMetricsStats(t *testing.T) {
	assert := assert.New(t)
	var err error

	taggerComponent := fxutil.Test[tagger.Mock](t, taggerimpl.MockModule())

	deps := fxutil.Test[testDeps](t, fx.Options(
		core.MockBundle(),
		fx.Supply(core.BundleParams{}),
		demultiplexerimpl.MockModule(),
		dogstatsd.Bundle(server.Params{Serverless: false}),
		defaultforwarder.MockModule(),
		fx.Provide(func() tagger.Component {
			return taggerComponent
		}),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	s := DsdStatsRuntimeSetting{
		ServerDebug: deps.Debug,
	}

	// runtime settings set/get underlying implementation

	// true string

	err = s.Set(deps.Config, "true", model.SourceDefault)
	assert.Nil(err)
	assert.Equal(deps.Debug.IsDebugEnabled(), true)
	v, err := s.Get(deps.Config)
	assert.Nil(err)
	assert.Equal(v, true)

	// false string

	err = s.Set(deps.Config, "false", model.SourceDefault)
	assert.Nil(err)
	assert.Equal(deps.Debug.IsDebugEnabled(), false)
	v, err = s.Get(deps.Config)
	assert.Nil(err)
	assert.Equal(v, false)

	// true boolean

	err = s.Set(deps.Config, true, model.SourceDefault)
	assert.Nil(err)
	assert.Equal(deps.Debug.IsDebugEnabled(), true)
	v, err = s.Get(deps.Config)
	assert.Nil(err)
	assert.Equal(v, true)

	// false boolean

	err = s.Set(deps.Config, false, model.SourceDefault)
	assert.Nil(err)
	assert.Equal(deps.Debug.IsDebugEnabled(), false)
	v, err = s.Get(deps.Config)
	assert.Nil(err)
	assert.Equal(v, false)

	// ensure the getter uses the value from the actual server

	deps.Debug.SetMetricStatsEnabled(true)
	v, err = s.Get(deps.Config)
	assert.Nil(err)
	assert.Equal(v, true)
}
