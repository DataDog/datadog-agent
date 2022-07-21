// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state/products/apmsampling"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

const maxRemoteTPS = 12377

func TestRemoteConfInit(t *testing.T) {
	assert := assert.New(t)
	// disabled by default
	assert.Nil(newRemoteRates(nil, 0, "6.0.0"))
	// subscription to subscriber fails
	assert.Nil(newRemoteRates(nil, 0, "6.0.0"))
	// todo:raphael mock grpc server
}

func newTestRemoteRates() *RemoteRates {
	return &RemoteRates{
		maxSigTPS:          maxRemoteTPS,
		samplers:           make(map[Signature]*remoteSampler),
		tpsVersion:         atomic.NewUint64(0),
		duplicateTargetTPS: atomic.NewUint64(0),
	}
}

func configGenerator(version uint64, rates apmsampling.APMSampling) map[string]state.APMSamplingConfig {
	return map[string]state.APMSamplingConfig{
		"testid": {
			Config: rates,
			Metadata: state.Metadata{
				ID:      "testid",
				Version: version,
			},
		},
	}
}

func TestRemoteTPSUpdate(t *testing.T) {
	assert := assert.New(t)

	type sampler struct {
		service   string
		env       string
		targetTPS float64
		mechanism apmsampling.SamplingMechanism
		rank      uint32
	}

	var testSteps = []struct {
		name             string
		ratesToApply     apmsampling.APMSampling
		countServices    []ServiceSignature
		expectedSamplers []sampler
		version          uint64
	}{
		{
			name: "first rates received",
			ratesToApply: apmsampling.APMSampling{
				TargetTPS: []apmsampling.TargetTPS{
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
			ratesToApply: apmsampling.APMSampling{
				TargetTPS: []apmsampling.TargetTPS{
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
		{
			name: "receive empty remote rates and above max",
			ratesToApply: apmsampling.APMSampling{
				TargetTPS: []apmsampling.TargetTPS{
					{
						Service: "keep",
						Value:   3718271,
					},
					{
						Service: "noop",
					},
				},
			},
			expectedSamplers: []sampler{
				{
					service:   "keep",
					targetTPS: maxRemoteTPS,
				},
			},
			version: 35,
		},
		{
			name: "keep highest rank",
			ratesToApply: apmsampling.APMSampling{
				TargetTPS: []apmsampling.TargetTPS{
					{
						Service:   "keep",
						Value:     10,
						Mechanism: 5,
						Rank:      3,
					},
					{
						Service:   "keep",
						Value:     10,
						Mechanism: 10,
						Rank:      10,
					},
					{
						Service:   "keep",
						Value:     10,
						Mechanism: 6,
						Rank:      6,
					},
				},
			},
			countServices: []ServiceSignature{{"keep", ""}},
			expectedSamplers: []sampler{
				{
					service:   "keep",
					targetTPS: 10,
					mechanism: 10,
					rank:      10,
				},
			},
		},
		{
			name: "duplicate",
			ratesToApply: apmsampling.APMSampling{
				TargetTPS: []apmsampling.TargetTPS{
					{
						Service: "keep",
						Value:   10,
						Rank:    3,
					},
					{
						Service: "keep",
						Value:   10,
						Rank:    3,
					},
				},
			},
			expectedSamplers: []sampler{
				{
					service:   "keep",
					targetTPS: 10,
					rank:      3,
				},
			},
		},
	}
	r := newTestRemoteRates()
	for _, step := range testSteps {
		t.Log(step.name)
		if step.ratesToApply.TargetTPS != nil {
			r.onUpdate(configGenerator(step.version, step.ratesToApply))
		}
		for _, s := range step.countServices {
			r.countWeightedSig(time.Now(), s.Hash(), 1)
		}

		assert.Len(r.samplers, len(step.expectedSamplers))

		for _, expectedS := range step.expectedSamplers {
			sig := ServiceSignature{Name: expectedS.service, Env: expectedS.env}.Hash()
			s, ok := r.samplers[sig]
			require.True(t, ok)
			root := &pb.Span{Metrics: map[string]float64{}}
			assert.Equal(expectedS.targetTPS, s.targetTPS.Load())
			assert.Equal(expectedS.mechanism, s.target.Mechanism)
			assert.Equal(expectedS.rank, s.target.Rank)
			r.countSample(root, sig)

			tpsTag, ok := root.Metrics[tagRemoteTPS]
			assert.True(ok)
			assert.Equal(expectedS.targetTPS, tpsTag)
			versionTag, ok := root.Metrics[tagRemoteVersion]
			assert.True(ok)
			assert.Equal(float64(step.version), versionTag)
		}
	}
}
