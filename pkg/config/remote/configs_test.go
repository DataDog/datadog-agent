package remote

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
)

func TestConfigsAPMSamplingUpdates(t *testing.T) {
	configs := newConfigs()
	samplingFile1 := pb.APMSampling{
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
			},
		},
	}
	samplingFile1Content, err := samplingFile1.MarshalMsg(nil)
	assert.NoError(t, err)

	update1 := configs.update([]data.Product{data.ProductAPMSampling}, configFiles{
		{
			pathMeta: data.PathMeta{
				Product:  data.ProductAPMSampling,
				ConfigID: "config_id1",
				Name:     "target_tps1",
			},
			version: 1,
			raw:     samplingFile1Content,
		},
	})
	expectedConfig1 := &APMSamplingConfig{
		Config: Config{
			ID:      "config_id1",
			Version: 1,
		},
		Rates: []pb.APMSampling{samplingFile1},
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
		Config: Config{
			ID:      "config_id2",
			Version: 2,
		},
		Rates: []pb.APMSampling{samplingFile2},
	}
	assert.Equal(t, update{apmSamplingUpdate: &APMSamplingUpdate{Config: expectedConfig2}}, update2)
}
