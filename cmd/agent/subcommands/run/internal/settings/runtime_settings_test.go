// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"testing"

	global "github.com/DataDog/datadog-agent/cmd/agent/dogstatsd"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/dogstatsd"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestDogstatsdMetricsStats(t *testing.T) {
	assert := assert.New(t)
	var err error

	opts := aggregator.DefaultAgentDemultiplexerOptions(nil)
	opts.DontStartForwarders = true
	demux := aggregator.InitAndStartAgentDemultiplexer(opts, "hostname")

	fxutil.Test(t, fx.Options(
		core.MockBundle,
		fx.Supply(core.BundleParams{}),
		fx.Supply(server.Params{
			Serverless: false,
		}),
		dogstatsd.Bundle,
	), func(server server.Component, debug serverDebug.Component) {

		global.DSD = server
		server.Start(demux)

		require.Nil(t, err)

		s := DsdStatsRuntimeSetting{
			ServerDebug: debug,
		}

		// runtime settings set/get underlying implementation

		// true string

		err = s.Set("true")
		assert.Nil(err)
		assert.Equal(debug.IsDebugEnabled(), true)
		v, err := s.Get()
		assert.Nil(err)
		assert.Equal(v, true)

		// false string

		err = s.Set("false")
		assert.Nil(err)
		assert.Equal(debug.IsDebugEnabled(), false)
		v, err = s.Get()
		assert.Nil(err)
		assert.Equal(v, false)

		// true boolean

		err = s.Set(true)
		assert.Nil(err)
		assert.Equal(debug.IsDebugEnabled(), true)
		v, err = s.Get()
		assert.Nil(err)
		assert.Equal(v, true)

		// false boolean

		err = s.Set(false)
		assert.Nil(err)
		assert.Equal(debug.IsDebugEnabled(), false)
		v, err = s.Get()
		assert.Nil(err)
		assert.Equal(v, false)

		// ensure the getter uses the value from the actual server

		debug.SetMetricStatsEnabled(true)
		v, err = s.Get()
		assert.Nil(err)
		assert.Equal(v, true)
	})
}
