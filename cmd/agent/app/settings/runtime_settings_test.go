// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"sync/atomic"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDogstatsdMetricsStats(t *testing.T) {
	assert := assert.New(t)
	var err error

	serializer := serializer.NewSerializer(common.Forwarder, nil)
	agg := aggregator.InitAggregator(serializer, nil, "")
	common.DSD, err = dogstatsd.NewServer(agg, nil)
	require.Nil(t, err)

	s := DsdStatsRuntimeSetting("dogstatsd_stats")

	// runtime settings set/get underlying implementation

	// true string

	err = s.Set("true")
	assert.Nil(err)
	assert.Equal(atomic.LoadUint64(&common.DSD.Debug.Enabled), uint64(1))
	v, err := s.Get()
	assert.Nil(err)
	assert.Equal(v, true)

	// false string

	err = s.Set("false")
	assert.Nil(err)
	assert.Equal(atomic.LoadUint64(&common.DSD.Debug.Enabled), uint64(0))
	v, err = s.Get()
	assert.Nil(err)
	assert.Equal(v, false)

	// true boolean

	err = s.Set(true)
	assert.Nil(err)
	assert.Equal(atomic.LoadUint64(&common.DSD.Debug.Enabled), uint64(1))
	v, err = s.Get()
	assert.Nil(err)
	assert.Equal(v, true)

	// false boolean

	err = s.Set(false)
	assert.Nil(err)
	assert.Equal(atomic.LoadUint64(&common.DSD.Debug.Enabled), uint64(0))
	v, err = s.Get()
	assert.Nil(err)
	assert.Equal(v, false)

	// ensure the getter uses the value from the actual server

	atomic.StoreUint64(&common.DSD.Debug.Enabled, 1)
	v, err = s.Get()
	assert.Nil(err)
	assert.Equal(v, true)
}
