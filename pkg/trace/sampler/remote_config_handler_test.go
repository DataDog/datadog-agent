// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"testing"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state/products/apmsampling"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/stretchr/testify/assert"
)

func TestRemoteConfigDisbled(t *testing.T) {
	// given that remote config is disabled
	conf := &config.AgentConfig{
		RemoteSamplingClient: nil,
	}

	// when we build the APM remote config handler
	a := NewAPMRemoteConfigHandler(conf, nil, nil)

	// then nil is returned
	assert.Nil(t, a)

	// when this nil handler is started
	a.Start()

	// then no error occurs
}

func TestConstructorRemoteConfigEnabled(t *testing.T) {
	testRemoteClient := TestRemoteClient{}

	// given that remote config is enabled
	conf := config.AgentConfig{
		RemoteSamplingClient: &testRemoteClient,
	}
	prioritySampler := PrioritySampler{}
	errorsSampler := ErrorsSampler{}

	// when we build the APM remote config handler
	a := NewAPMRemoteConfigHandler(&conf, &prioritySampler, &errorsSampler)

	assert.Equal(t, &conf, a.conf)
	assert.Equal(t, &prioritySampler, a.prioritySampler)
	assert.Equal(t, &errorsSampler, a.errorsSampler)

	// then remote rates are enabled in the priority sampler
	assert.NotNil(t, a.prioritySampler.remoteRates)

	initialStartCounter := testRemoteClient.StartCallCounter
	initialRegisterCounter := testRemoteClient.RegisterAPMUpdateCallCounter

	// when the handler is started
	a.Start()

	// then the remote config client is strated and a listener registered
	assert.Equal(t, initialStartCounter+1, testRemoteClient.StartCallCounter)
	assert.Equal(t, initialRegisterCounter+1, testRemoteClient.RegisterAPMUpdateCallCounter)
}

func TestToggleRareSampler(t *testing.T) {
	// given an enabled rare sampler
	testRemoteClient := TestRemoteClient{}
	conf := config.AgentConfig{
		RemoteSamplingClient: &testRemoteClient,
		RareSamplerDisabled:  false,
	}
	prioritySampler := PrioritySampler{}
	errorsSampler := ErrorsSampler{}
	a := NewAPMRemoteConfigHandler(&conf, &prioritySampler, &errorsSampler)
	a.Start()

	// when an unset remote config value for rare sampler is received
	update := state.APMSamplingConfig{
		Config: apmsampling.APMSampling{
			RareSamplerConfig: apmsampling.RareSamplerConfigUnset,
		},
	}
	a.onUpdate(map[string]state.APMSamplingConfig{"ignore": update})
	// then the rare sampler stays enabled
	assert.False(t, conf.RareSamplerDisabled)

	// when a "disabled" remote config value is received
	update.Config.RareSamplerConfig = apmsampling.RareSamplerConfigDisabled
	a.onUpdate(map[string]state.APMSamplingConfig{"ignore": update})
	// then the rare sampler is disabled
	assert.True(t, conf.RareSamplerDisabled)

	// when an unset remote config value is received again
	update.Config.RareSamplerConfig = apmsampling.RareSamplerConfigUnset
	a.onUpdate(map[string]state.APMSamplingConfig{"ignore": update})
	// then the rare sampler stays disabled
	assert.True(t, conf.RareSamplerDisabled)

	// when a "enabled" remote config value is received
	update.Config.RareSamplerConfig = apmsampling.RareSamplerConfigEnabled
	a.onUpdate(map[string]state.APMSamplingConfig{"ignore": update})
	// then the rare sampler is enabled
	assert.False(t, conf.RareSamplerDisabled)
}

func TestUpdateErrorsSamplerTPS(t *testing.T) {
	// given an errors sampler with tartget TPS 1
	testRemoteClient := TestRemoteClient{}
	conf := config.AgentConfig{
		RemoteSamplingClient: &testRemoteClient,
	}
	prioritySampler := PrioritySampler{}
	errorsSampler := ErrorsSampler{ScoreSampler: ScoreSampler{Sampler: &Sampler{targetTPS: atomic.NewFloat64(1)}}}
	a := NewAPMRemoteConfigHandler(&conf, &prioritySampler, &errorsSampler)
	a.Start()

	// when a remote config value for errors TPS is received
	update := state.APMSamplingConfig{
		Config: apmsampling.APMSampling{
			ErrorsSamplerConfig: &apmsampling.ErrorSamplerConfig{
				TargetTPS: 2,
			},
		},
	}
	a.onUpdate(map[string]state.APMSamplingConfig{"ignore": update})
	// then the errors sampler target TPS is updated
	assert.Equal(t, 2., errorsSampler.ScoreSampler.Sampler.targetTPS.Load())

	// when a nil remote config value is received
	update.Config.ErrorsSamplerConfig = nil
	a.onUpdate(map[string]state.APMSamplingConfig{"ignore": update})
	// then the target TPS does not change
	assert.Equal(t, 2., errorsSampler.ScoreSampler.Sampler.targetTPS.Load())

	// when a zero remote config value is received
	update.Config.ErrorsSamplerConfig = &apmsampling.ErrorSamplerConfig{
		TargetTPS: 0,
	}
	a.onUpdate(map[string]state.APMSamplingConfig{"ignore": update})
	// then the target TPS is zeroed
	assert.Equal(t, 0., errorsSampler.ScoreSampler.Sampler.targetTPS.Load())
}

type TestRemoteClient struct {
	StartCallCounter             int32
	RegisterAPMUpdateCallCounter int32
}

func (r *TestRemoteClient) Start() { r.StartCallCounter++ }
func (r *TestRemoteClient) Close() {}
func (r *TestRemoteClient) RegisterAPMUpdate(func(update map[string]state.APMSamplingConfig)) {
	r.RegisterAPMUpdateCallCounter++
}
