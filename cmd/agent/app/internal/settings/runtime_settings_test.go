// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
)

func TestDogstatsdMetricsStats(t *testing.T) {
	assert := assert.New(t)
	var err error

	opts := aggregator.DefaultAgentDemultiplexerOptions(nil)
	opts.DontStartForwarders = true
	demux := aggregator.InitAndStartAgentDemultiplexer(opts, "hostname")
	common.DSD, err = dogstatsd.NewServer(demux, false)
	require.Nil(t, err)

	s := DsdStatsRuntimeSetting("dogstatsd_stats")

	// runtime settings set/get underlying implementation

	// true string

	err = s.Set("true")
	assert.Nil(err)
	assert.Equal(common.DSD.Debug.Enabled.Load(), true)
	v, err := s.Get()
	assert.Nil(err)
	assert.Equal(v, true)

	// false string

	err = s.Set("false")
	assert.Nil(err)
	assert.Equal(common.DSD.Debug.Enabled.Load(), false)
	v, err = s.Get()
	assert.Nil(err)
	assert.Equal(v, false)

	// true boolean

	err = s.Set(true)
	assert.Nil(err)
	assert.Equal(common.DSD.Debug.Enabled.Load(), true)
	v, err = s.Get()
	assert.Nil(err)
	assert.Equal(v, true)

	// false boolean

	err = s.Set(false)
	assert.Nil(err)
	assert.Equal(common.DSD.Debug.Enabled.Load(), false)
	v, err = s.Get()
	assert.Nil(err)
	assert.Equal(v, false)

	// ensure the getter uses the value from the actual server

	common.DSD.Debug.Enabled.Store(true)
	v, err = s.Get()
	assert.Nil(err)
	assert.Equal(v, true)
}
