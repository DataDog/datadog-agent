package sampler

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteConfInit(t *testing.T) {
	assert := assert.New(t)
	// disabled by default
	assert.Nil(newRemoteRates())
	// subscription to subscriber fails
	old := os.Getenv("DD_APM_FEATURES")
	os.Setenv("DD_APM_FEATURES", "remote_rates")
	assert.Nil(newRemoteRates())
	os.Setenv("DD_APM_FEATURES", old)
	// todo:raphael mock grpc server
}

func newTestRemoteRates() *RemoteRates {
	return &RemoteRates{
		samplers: make(map[Signature]*Sampler),

		exit:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
}

func configGenerator(version uint64, rates pb.APMSampling) *pbgo.ConfigResponse {
	raw, _ := rates.MarshalMsg(nil)
	return &pbgo.ConfigResponse{
		ConfigDelegatedTargetVersion: version,
		TargetFiles:                  []*pbgo.File{{Raw: raw}},
	}
}

func TestRemoteTPSUpdate(t *testing.T) {
	assert := assert.New(t)

	type sampler struct {
		service   string
		env       string
		targetTPS float64
	}

	var testSteps = []struct {
		name             string
		ratesToApply     pb.APMSampling
		countServices    []ServiceSignature
		expectedSamplers []sampler
		version          uint64
	}{
		{
			name: "first rates received",
			ratesToApply: pb.APMSampling{
				TargetTps: []pb.TargetTPS{
					{
						Service: "willBeRemoved",
						Value:   3.2,
					},
					{
						Service: "willBeRemoved",
						Env:     "env2",
						Value:   33,
					},
					{
						Service: "keep",
						Value:   1,
					},
				},
			},
			version: 30,
		},
		{
			name: "enable a sampler after counting a matching service",
			countServices: []ServiceSignature{
				{
					Name: "willBeRemoved",
				},
			},
			expectedSamplers: []sampler{
				{
					service:   "willBeRemoved",
					targetTPS: 3.2,
				},
			},
			version: 30,
		},
		{
			name: "nothing happens when counting a service not set remotely",
			countServices: []ServiceSignature{
				{
					Name: "no remote tps",
				},
			},
			expectedSamplers: []sampler{
				{
					service:   "willBeRemoved",
					targetTPS: 3.2,
				},
			},
			version: 30,
		},
		{
			name: "add 2 more samplers",
			countServices: []ServiceSignature{
				{
					Name: "keep",
				},
				{
					Name: "willBeRemoved",
					Env:  "env2",
				},
			},
			expectedSamplers: []sampler{
				{
					service:   "willBeRemoved",
					targetTPS: 3.2,
				},
				{
					service:   "willBeRemoved",
					env:       "env2",
					targetTPS: 33,
				},
				{
					service:   "keep",
					targetTPS: 1,
				},
			},
			version: 30,
		},
		{
			name: "receive new remote rates, non matching samplers are trimmed",
			ratesToApply: pb.APMSampling{
				TargetTps: []pb.TargetTPS{
					{
						Service: "keep",
						Value:   27,
					},
				},
			},
			expectedSamplers: []sampler{
				{
					service:   "keep",
					targetTPS: 27,
				},
			},
			version: 35,
		},
	}
	r := newTestRemoteRates()
	for _, step := range testSteps {
		t.Log(step.name)
		if step.ratesToApply.TargetTps != nil {
			r.loadNewConfig(configGenerator(step.version, step.ratesToApply))
		}
		for _, s := range step.countServices {
			r.CountSignature(s.Hash())
		}

		assert.Len(r.samplers, len(step.expectedSamplers))

		for _, expectedS := range step.expectedSamplers {
			sig := ServiceSignature{Name: expectedS.service, Env: expectedS.env}.Hash()
			s, ok := r.samplers[sig]
			require.True(t, ok)
			root := &pb.Span{Metrics: map[string]float64{}}
			assert.Equal(expectedS.targetTPS, s.targetTPS.Load())
			r.CountSample(root, sig)

			tpsTag, ok := root.Metrics[tagRemoteTPS]
			assert.True(ok)
			assert.Equal(expectedS.targetTPS, tpsTag)
			versionTag, ok := root.Metrics[tagRemoteVersion]
			assert.True(ok)
			assert.Equal(float64(step.version), versionTag)
		}
	}
}
