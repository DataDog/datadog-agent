// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewDynamicConfig(t *testing.T) {
	assert := assert.New(t)

	dc := NewDynamicConfig()
	assert.NotNil(dc)

	rates := map[ServiceSignature]float64{
		{"myservice", "myenv"}: 0.5,
	}

	// Not doing a complete test of the different components of dynamic config,
	// but still assessing it can do the bare minimum once initialized.
	dc.RateByService.SetAll(rates)
	state := dc.RateByService.GetNewState("")
	assert.Equal(map[string]float64{"service:myservice,env:myenv": 0.5}, state.Rates)
	assert.NotEqual("", state.Version)

	state = dc.RateByService.GetNewState(state.Version)
	assert.Equal(map[string]float64(nil), state.Rates)
}

func TestRateByServiceGetSet(t *testing.T) {
	var rbc RateByService
	for i, tc := range []struct {
		in  map[ServiceSignature]float64
		out State
	}{
		{
			in: map[ServiceSignature]float64{
				{}: 0.1,
			},
			out: State{
				Rates: map[string]float64{
					"service:,env:": 0.1,
				},
			},
		},
		{
			in: map[ServiceSignature]float64{
				{}:                  0.3,
				{"mcnulty", "dev"}:  0.2,
				{"postgres", "dev"}: 0.1,
			},
			out: State{
				Rates: map[string]float64{
					"service:,env:":            0.3,
					"service:mcnulty,env:dev":  0.2,
					"service:postgres,env:dev": 0.1,
				},
			},
		},
		{
			in: map[ServiceSignature]float64{
				{}: 1,
			},
			out: State{
				Rates: map[string]float64{
					"service:,env:": 1,
				},
			},
		},
		{
			out: State{
				Rates: map[string]float64{},
			},
		},
		{
			in: map[ServiceSignature]float64{
				{}: 0.2,
			},
			out: State{
				Rates: map[string]float64{
					"service:,env:": 0.2,
				},
			},
		},
	} {
		rbc.SetAll(tc.in)
		state := rbc.GetNewState("")
		tc.out.Version = state.Version
		assert.Equal(t, tc.out, state, strconv.Itoa(i))
	}
}

func TestRateByServiceLimits(t *testing.T) {
	assert := assert.New(t)

	var rbc RateByService
	rbc.SetAll(map[ServiceSignature]float64{
		{"high", ""}: 2,
		{"low", ""}:  -1,
	})
	assert.Equal(map[string]float64{"service:high,env:": 1, "service:low,env:": 0}, rbc.GetNewState("").Rates)
}

func TestRateByServiceDefaults(t *testing.T) {
	rbc := RateByService{}
	rbc.SetAll(map[ServiceSignature]float64{
		{"one", "prod"}: 0.5,
		{"two", "test"}: 0.4,
	})
	assert.Equal(t, map[string]float64{
		"service:one,env:prod": 0.5,
		"service:two,env:test": 0.4,
	}, rbc.GetNewState("").Rates)
}

func TestVersionChanges(t *testing.T) {
	rbc := RateByService{}
	rates := map[ServiceSignature]float64{
		{"one", "prod"}:   0.5,
		{"two", "test"}:   0.4,
		{"three", "test"}: 0.4,
		{"four", "test"}:  0.4,
	}

	previousVersion := rbc.GetNewState("").Version
	rbc.SetAll(rates)
	newVersion := rbc.GetNewState("").Version
	assert.Equal(t, "", previousVersion)

	// received the same rates
	previousVersion = newVersion
	rbc.SetAll(rates)
	newVersion = rbc.GetNewState("").Version
	assert.Equal(t, previousVersion, newVersion)

	// received slightly different rates
	previousVersion = newVersion
	rates[ServiceSignature{"one", "prod"}] = 0.4
	rbc.SetAll(rates)
	newVersion = rbc.GetNewState("").Version
	assert.NotEqual(t, previousVersion, newVersion)

	// received an extra rate
	previousVersion = newVersion
	rates[ServiceSignature{"newService", "prod"}] = 0.99
	rbc.SetAll(rates)
	newVersion = rbc.GetNewState("").Version
	assert.NotEqual(t, previousVersion, newVersion)

	// received fewer rates
	previousVersion = newVersion
	delete(rates, ServiceSignature{"newService", "prod"})
	rbc.SetAll(rates)
	newVersion = rbc.GetNewState("").Version
	assert.NotEqual(t, previousVersion, newVersion)

	// the response should be empty, because both tracer and agent have the same version
	state := rbc.GetNewState(newVersion)
	assert.Equal(t, State{Version: newVersion}, state)
}

func TestRateByServiceConcurrency(t *testing.T) {
	assert := assert.New(t)

	var rbc RateByService

	const n = 1000
	var wg sync.WaitGroup
	wg.Add(2)

	rbc.SetAll(map[ServiceSignature]float64{{"mcnulty", "test"}: 1})
	go func() {
		for i := 0; i < n; i++ {
			rate := float64(i) / float64(n)
			rbc.SetAll(map[ServiceSignature]float64{{"mcnulty", "test"}: rate})
		}
		wg.Done()
	}()
	go func() {
		for i := 0; i < n; i++ {
			rates := rbc.GetNewState("").Rates
			_, ok := rates["service:mcnulty,env:test"]
			assert.True(ok, "key should be here")
		}
		wg.Done()
	}()
}

func benchRBSGetAll(sigs map[ServiceSignature]float64) func(*testing.B) {
	return func(b *testing.B) {
		rbs := &RateByService{}
		rbs.SetAll(sigs)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			rbs.GetNewState("")
		}
	}
}

func benchRBSSetAll(sigs map[ServiceSignature]float64) func(*testing.B) {
	return func(b *testing.B) {
		rbs := &RateByService{}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			rbs.SetAll(sigs)
		}
	}
}

func BenchmarkRateByService(b *testing.B) {
	sigs := map[ServiceSignature]float64{
		{}:                 0.2,
		{"two", "test"}:    0.4,
		{"three", "test"}:  0.33,
		{"one", "prod"}:    0.12,
		{"five", "test"}:   0.8,
		{"six", "staging"}: 0.9,
	}

	b.Run("GetAll", func(b *testing.B) {
		for i := 1; i <= len(sigs); i++ {
			// take first i elements
			testSigs := make(map[ServiceSignature]float64, i)
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
			testSigs := make(map[ServiceSignature]float64, i)
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
