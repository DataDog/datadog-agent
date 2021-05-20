// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/export/sampler"

	"github.com/stretchr/testify/assert"
)

// TestCatalogRegression is a regression tests ensuring that there is no race
// occurring when registering entries in the catalog in parallel to obtaining
// the rates by service map.
func TestCatalogRegression(t *testing.T) {
	cat := newServiceLookup()
	n := 100

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			cat.register(sampler.ServiceSignature{})
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			cat.ratesByService(map[sampler.Signature]float64{
				sampler.ServiceSignature{}.Hash():                            0.3,
				sampler.ServiceSignature{Name: "web", Env: "staging"}.Hash(): 0.4,
			}, 0.2)
		}
	}()

	wg.Wait()
}

func TestServiceSignatureString(t *testing.T) {
	assert := assert.New(t)

	assert.Equal(defaultServiceRateKey, sampler.ServiceSignature{}.String())
	assert.Equal("service:mcnulty,env:test", sampler.ServiceSignature{Name: "mcnulty", Env: "test"}.String())
}

func TestNewServiceLookup(t *testing.T) {
	cat := newServiceLookup()
	assert.NotNil(t, cat.lookup)
}

func TestServiceKeyCatalogRegister(t *testing.T) {
	assert := assert.New(t)

	cat := newServiceLookup()
	s := getTestPrioritySampler()

	_, root1 := getTestTraceWithService(t, "service1", s)
	sig1 := cat.register(sampler.ServiceSignature{Name: root1.Service, Env: defaultEnv})
	assert.Equal(
		map[sampler.ServiceSignature]sampler.Signature{
			{Name: "service1", Env: "none"}: sig1,
		},
		cat.lookup,
	)

	_, root2 := getTestTraceWithService(t, "service2", s)
	sig2 := cat.register(sampler.ServiceSignature{Name: root2.Service, Env: defaultEnv})
	assert.Equal(
		map[sampler.ServiceSignature]sampler.Signature{
			{Name: "service1", Env: "none"}: sig1,
			{Name: "service2", Env: "none"}: sig2,
		},
		cat.lookup,
	)
}

func TestServiceKeyCatalogRatesByService(t *testing.T) {
	assert := assert.New(t)

	cat := newServiceLookup()
	s := getTestPrioritySampler()

	_, root1 := getTestTraceWithService(t, "service1", s)
	sig1 := cat.register(sampler.ServiceSignature{Name: root1.Service, Env: defaultEnv})
	_, root2 := getTestTraceWithService(t, "service2", s)
	sig2 := cat.register(sampler.ServiceSignature{Name: root2.Service, Env: defaultEnv})

	rates := map[sampler.Signature]float64{
		sig1: 0.3,
		sig2: 0.7,
	}
	const totalRate = 0.2

	rateByService := cat.ratesByService(rates, totalRate)
	assert.Equal(map[sampler.ServiceSignature]float64{
		{Name: "service1", Env: "none"}: 0.3,
		{Name: "service2", Env: "none"}: 0.7,
		{}:                              0.2,
	}, rateByService)

	delete(rates, sig1)

	rateByService = cat.ratesByService(rates, totalRate)
	assert.Equal(map[sampler.ServiceSignature]float64{
		{Name: "service2", Env: "none"}: 0.7,
		{}:                              0.2,
	}, rateByService)

	delete(rates, sig2)

	rateByService = cat.ratesByService(rates, totalRate)
	assert.Equal(map[sampler.ServiceSignature]float64{
		{}: 0.2,
	}, rateByService)
}
