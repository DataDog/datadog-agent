package remote

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/remote/util"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
)

func TestConfigsAPMSamplingUpdates(t *testing.T) {
	configs := newConfigs()
	samplingFile1 := pb.APMSampling{
		TargetTps: []pb.TargetTPS{
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

	update1 := configs.update([]pbgo.Product{pbgo.Product_APM_SAMPLING}, configFiles{
		{
			pathMeta: util.PathMeta{
				Product:  pbgo.Product_APM_SAMPLING,
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
		APMSampling: samplingFile1,
	}
	assert.Equal(t, update{apmSamplingUpdate: &APMSamplingUpdate{Config: expectedConfig1}}, update1)

	samplingFile2 := pb.APMSampling{
		TargetTps: []pb.TargetTPS{
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
	update2 := configs.update([]pbgo.Product{pbgo.Product_APM_SAMPLING}, configFiles{
		{
			pathMeta: util.PathMeta{
				Product:  pbgo.Product_APM_SAMPLING,
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
		APMSampling: samplingFile2,
	}
	assert.Equal(t, update{apmSamplingUpdate: &APMSamplingUpdate{Config: expectedConfig2}}, update2)
}
