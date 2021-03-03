// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"strings"
	"sync/atomic"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type runtimeTestSetting struct {
	value int
}

func (t *runtimeTestSetting) Name() string {
	return "name"
}

func (t *runtimeTestSetting) Description() string {
	return "desc"
}

func (t *runtimeTestSetting) Get() (interface{}, error) {
	return t.value, nil
}

func (t *runtimeTestSetting) Set(v interface{}) error {
	t.value = v.(int)
	return nil
}

func (t *runtimeTestSetting) Hidden() bool {
	return false
}

func setupConf() config.Config {
	conf := config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	config.InitConfig(conf)
	return conf
}

func cleanRuntimeSetting() {
	runtimeSettings = make(map[string]RuntimeSetting)
}

func TestRuntimeSettings(t *testing.T) {
	cleanRuntimeSetting()
	runtimeSetting := runtimeTestSetting{1}

	err := registerRuntimeSetting(&runtimeSetting)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(RuntimeSettings()))

	v, err := GetRuntimeSetting(runtimeSetting.Name())
	assert.Nil(t, err)
	assert.Equal(t, runtimeSetting.value, v)

	err = SetRuntimeSetting(runtimeSetting.Name(), 123)
	assert.Nil(t, err)

	v, err = GetRuntimeSetting(runtimeSetting.Name())
	assert.Nil(t, err)
	assert.Equal(t, 123, v)

	err = registerRuntimeSetting(&runtimeSetting)
	assert.NotNil(t, err)
	assert.Equal(t, "duplicated settings detected", err.Error())
}

func TestLogLevel(t *testing.T) {
	cleanRuntimeSetting()
	config.SetupLogger("TEST", "debug", "", "", true, true, true)

	ll := logLevelRuntimeSetting("log_level")
	assert.Equal(t, "log_level", ll.Name())

	err := ll.Set("off")
	assert.Nil(t, err)

	v, err := ll.Get()
	assert.Equal(t, "off", v)
	assert.Nil(t, err)

	err = ll.Set("WARNING")
	assert.Nil(t, err)

	v, err = ll.Get()
	assert.Equal(t, "warn", v)
	assert.Nil(t, err)

	err = ll.Set("invalid")
	assert.NotNil(t, err)
	assert.Equal(t, "unknown log level: invalid", err.Error())

	v, err = ll.Get()
	assert.Equal(t, "warn", v)
	assert.Nil(t, err)
}

func TestDogstatsdMetricsStats(t *testing.T) {
	assert := assert.New(t)
	var err error

	serializer := serializer.NewSerializer(common.Forwarder, nil)
	agg := aggregator.InitAggregator(serializer, "")
	common.DSD, err = dogstatsd.NewServer(agg, nil)
	require.Nil(t, err)

	cleanRuntimeSetting()

	s := dsdStatsRuntimeSetting("dogstatsd_stats")

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

func TestProfiling(t *testing.T) {
	cleanRuntimeSetting()
	setupConf()

	ll := profilingRuntimeSetting("profiling")
	assert.Equal(t, "profiling", ll.Name())

	err := ll.Set("false")
	assert.Nil(t, err)

	v, err := ll.Get()
	assert.Equal(t, false, v)
	assert.Nil(t, err)

	err = ll.Set("on")
	assert.NotNil(t, err)
}
