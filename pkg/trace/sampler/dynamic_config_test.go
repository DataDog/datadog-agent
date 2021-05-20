// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"strconv"
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/export/sampler"

	"github.com/stretchr/testify/assert"
)

func TestNewDynamicConfig(t *testing.T) {
	assert := assert.New(t)

	dc := NewDynamicConfig("none")
	assert.NotNil(dc)

	rates := map[sampler.ServiceSignature]float64{
		{Name: "myservice", Env: "myenv"}: 0.5,
	}

	// Not doing a complete test of the different components of dynamic config,
	// but still assessing it can do the bare minimum once initialized.
	dc.RateByService.SetAll(rates)
	rbs := dc.RateByService.GetAll()
	assert.Equal(map[string]float64{"service:myservice,env:myenv": 0.5}, rbs)
}

func TestRateByServiceGetSet(t *testing.T) {
	var rbc RateByService
	for i, tc := range []struct {
		in  map[sampler.ServiceSignature]float64
		out map[string]float64
	}{
		{
			in: map[sampler.ServiceSignature]float64{
				{}: 0.1,
			},
			out: map[string]float64{
				"service:,env:": 0.1,
			},
		},
		{
			in: map[sampler.ServiceSignature]float64{
				{}:                             0.3,
				{Name: "mcnulty", Env: "dev"}:  0.2,
				{Name: "postgres", Env: "dev"}: 0.1,
			},
			out: map[string]float64{
				"service:,env:":            0.3,
				"service:mcnulty,env:dev":  0.2,
				"service:postgres,env:dev": 0.1,
			},
		},
		{
			in: map[sampler.ServiceSignature]float64{
				{}: 1,
			},
			out: map[string]float64{
				"service:,env:": 1,
			},
		},
		{
			out: map[string]float64{},
		},
		{
			in: map[sampler.ServiceSignature]float64{
				{}: 0.2,
			},
			out: map[string]float64{
				"service:,env:": 0.2,
			},
		},
	} {
		rbc.SetAll(tc.in)
		assert.Equal(t, tc.out, rbc.GetAll(), strconv.Itoa(i))
	}
}

func TestRateByServiceLimits(t *testing.T) {
	assert := assert.New(t)

	var rbc RateByService
	rbc.SetAll(map[sampler.ServiceSignature]float64{
		{Name: "high", Env: ""}: 2,
		{Name: "low", Env: ""}:  -1,
	})
	assert.Equal(map[string]float64{"service:high,env:": 1, "service:low,env:": 0}, rbc.GetAll())
}

func TestRateByServiceDefaults(t *testing.T) {
	rbc := RateByService{defaultEnv: "test"}
	rbc.SetAll(map[sampler.ServiceSignature]float64{
		{Name: "one", Env: "prod"}: 0.5,
		{Name: "two", Env: "test"}: 0.4,
	})
	assert.Equal(t, map[string]float64{
		"service:one,env:prod": 0.5,
		"service:two,env:test": 0.4,
		"service:two,env:":     0.4,
	}, rbc.GetAll())
}

func TestRateByServiceConcurrency(t *testing.T) {
	assert := assert.New(t)

	var rbc RateByService

	const n = 1000
	var wg sync.WaitGroup
	wg.Add(2)

	rbc.SetAll(map[sampler.ServiceSignature]float64{{Name: "mcnulty", Env: "test"}: 1})
	go func() {
		for i := 0; i < n; i++ {
			rate := float64(i) / float64(n)
			rbc.SetAll(map[sampler.ServiceSignature]float64{{Name: "mcnulty", Env: "test"}: rate})
		}
		wg.Done()
	}()
	go func() {
		for i := 0; i < n; i++ {
			rates := rbc.GetAll()
			_, ok := rates["service:mcnulty,env:test"]
			assert.True(ok, "key should be here")
		}
		wg.Done()
	}()
}

func benchRBSGetAll(sigs map[sampler.ServiceSignature]float64) func(*testing.B) {
	return func(b *testing.B) {
		rbs := &RateByService{defaultEnv: "test"}
		rbs.SetAll(sigs)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			rbs.GetAll()
		}
	}
}

func benchRBSSetAll(sigs map[sampler.ServiceSignature]float64) func(*testing.B) {
	return func(b *testing.B) {
		rbs := &RateByService{defaultEnv: "test"}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			rbs.SetAll(sigs)
		}
	}
}

func BenchmarkRateByService(b *testing.B) {
	sigs := map[sampler.ServiceSignature]float64{
		{}:                            0.2,
		{Name: "two", Env: "test"}:    0.4,
		{Name: "three", Env: "test"}:  0.33,
		{Name: "one", Env: "prod"}:    0.12,
		{Name: "five", Env: "test"}:   0.8,
		{Name: "six", Env: "staging"}: 0.9,
	}

	b.Run("GetAll", func(b *testing.B) {
		for i := 1; i <= len(sigs); i++ {
			// take first i elements
			testSigs := make(map[sampler.ServiceSignature]float64, i)
			var j int
			for k, v := range sigs {
				j++
				testSigs[k] = v
				if j == i {
					break
				}
			}
			b.Run(strconv.Itoa(i), benchRBSGetAll(testSigs))
		}
	})

	b.Run("SetAll", func(b *testing.B) {
		for i := 1; i <= len(sigs); i++ {
			// take first i elements
			testSigs := make(map[sampler.ServiceSignature]float64, i)
			var j int
			for k, v := range sigs {
				j++
				testSigs[k] = v
				if j == i {
					break
				}
			}
			b.Run(strconv.Itoa(i), benchRBSSetAll(testSigs))
		}
	})
}
