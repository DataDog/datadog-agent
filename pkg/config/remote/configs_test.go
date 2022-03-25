package remote

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
)

func TestConfigsAPMSamplingUpdates(t *testing.T) {
	configs := newConfigs()
	samplingFile1a := pb.APMSampling{
		TargetTPS: []pb.TargetTPS{
			{
				Env:     "env1",
				Service: "service1",
				Value:   1,
			},
			{
				Env:     "env2",
				Service: "service2",
				Value:   2,
				Rank:    2,
			},
		},
	}
	samplingFile1b := pb.APMSampling{
		TargetTPS: []pb.TargetTPS{
			{
				Env:     "env1",
				Service: "service1",
				Value:   4.7,
				Rank:    2,
			},
			{
				Env:     "env2",
				Service: "service2",
				Value:   2,
			},
			{
				Env:     "env3",
				Service: "service3",
				Value:   7,
			},
		},
	}
	samplingFile1Content1, err := samplingFile1a.MarshalMsg(nil)
	assert.NoError(t, err)
	samplingFile1Content2, err := samplingFile1b.MarshalMsg(nil)
	assert.NoError(t, err)

	update1 := configs.update([]data.Product{data.ProductAPMSampling}, configFiles{
		{
			pathMeta: data.PathMeta{
				Product:  data.ProductAPMSampling,
				ConfigID: "config_id1",
				Name:     "target_tps1",
			},
			version: 1,
			raw:     samplingFile1Content1,
		},
		{
			pathMeta: data.PathMeta{
				Product:  data.ProductAPMSampling,
				ConfigID: "config_id2",
				Name:     "target_tps2",
			},
			version: 2,
			raw:     samplingFile1Content2,
		},
	})
	expectedConfig1 := &APMSamplingConfig{
		Configs: map[string]Config{
			"config_id1": {
				ID:      "config_id1",
				Version: 1,
			},
			"config_id2": {
				ID:      "config_id2",
				Version: 2,
			},
		},
		Rates: []pb.APMSampling{samplingFile1a, samplingFile1b},
	}
	assert.Equal(t, update{apmSamplingUpdate: &APMSamplingUpdate{Config: expectedConfig1}}, update1)

	samplingFile2 := pb.APMSampling{
		TargetTPS: []pb.TargetTPS{
			{
				Env:     "env3",
				Service: "service3",
				Value:   3,
			},
			{
				Env:     "env4",
				Service: "service4",
				Value:   4,
			},
		},
	}
	samplingFile2Content, err := samplingFile2.MarshalMsg(nil)
	assert.NoError(t, err)
	update2 := configs.update([]data.Product{data.ProductAPMSampling}, configFiles{
		{
			pathMeta: data.PathMeta{
				Product:  data.ProductAPMSampling,
				ConfigID: "config_id2",
				Name:     "target_tps2",
			},
			version: 2,
			raw:     samplingFile2Content,
		},
	})
	expectedConfig2 := &APMSamplingConfig{
		Configs: map[string]Config{
			"config_id2": {
				ID:      "config_id2",
				Version: 2,
			},
		},
		Rates: []pb.APMSampling{samplingFile2},
	}
	assert.Equal(t, update{apmSamplingUpdate: &APMSamplingUpdate{Config: expectedConfig2}}, update2)
}

func TestShouldUpdate(t *testing.T) {
	c := newApmSamplingConfigs()
	assert.True(t, c.shouldUpdate(nil))
	assert.True(t, c.shouldUpdate(map[string]configFiles{}))
	assert.True(t, c.shouldUpdate(map[string]configFiles{
		"": {},
	}))

	c.config = &APMSamplingConfig{
		Configs: map[string]Config{
			"id1": {
				ID:      "id1",
				Version: 1,
			},
			"id2": {
				ID:      "id2",
				Version: 2,
			},
		},
	}
	assert.True(t, c.shouldUpdate(nil))
	assert.True(t, c.shouldUpdate(map[string]configFiles{}))
	assert.True(t, c.shouldUpdate(map[string]configFiles{
		"": {},
	}))
	assert.False(t, c.shouldUpdate(map[string]configFiles{
		"id1": {{version: 1}},
		"id2": {{version: 1}},
	}))
	assert.False(t, c.shouldUpdate(map[string]configFiles{
		"id1": {{version: 1}},
		"id2": {{version: 2}},
	}))
	assert.True(t, c.shouldUpdate(map[string]configFiles{
		"id1": {{version: 1}},
		"id2": {{version: 2}},
		"id3": {{version: 1}},
	}))
	assert.True(t, c.shouldUpdate(map[string]configFiles{
		"id1": {{version: 1}},
		"id2": {{version: 3}},
	}))
}
