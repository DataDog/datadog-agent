// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package sampler

import (
	"sync"
	"testing"

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
			cat.register(ServiceSignature{})
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			cat.ratesByService(map[Signature]float64{
				ServiceSignature{}.Hash():                 0.3,
				ServiceSignature{"web", "staging"}.Hash(): 0.4,
			}, 0.2)
		}
	}()

	wg.Wait()
}

func TestServiceSignatureString(t *testing.T) {
	assert := assert.New(t)

	assert.Equal(defaultServiceRateKey, ServiceSignature{}.String())
	assert.Equal("service:mcnulty,env:test", ServiceSignature{"mcnulty", "test"}.String())
}

func TestNewServiceLookup(t *testing.T) {
	cat := newServiceLookup()
	assert.NotNil(t, cat.lookup)
}

func TestServiceKeyCatalogRegister(t *testing.T) {
	assert := assert.New(t)

	cat := newServiceLookup()
	s := getTestPriorityEngine()

	_, root1 := getTestTraceWithService(t, "service1", s)
	sig1 := cat.register(ServiceSignature{root1.Service, defaultEnv})
	assert.Equal(
		map[ServiceSignature]Signature{
			{"service1", "none"}: sig1,
		},
		cat.lookup,
	)

	_, root2 := getTestTraceWithService(t, "service2", s)
	sig2 := cat.register(ServiceSignature{root2.Service, defaultEnv})
	assert.Equal(
		map[ServiceSignature]Signature{
			{"service1", "none"}: sig1,
			{"service2", "none"}: sig2,
		},
		cat.lookup,
	)
}

func TestServiceKeyCatalogRatesByService(t *testing.T) {
	assert := assert.New(t)

	cat := newServiceLookup()
	s := getTestPriorityEngine()

	_, root1 := getTestTraceWithService(t, "service1", s)
	sig1 := cat.register(ServiceSignature{root1.Service, defaultEnv})
	_, root2 := getTestTraceWithService(t, "service2", s)
	sig2 := cat.register(ServiceSignature{root2.Service, defaultEnv})

	rates := map[Signature]float64{
		sig1: 0.3,
		sig2: 0.7,
	}
	const totalRate = 0.2

	rateByService := cat.ratesByService(rates, totalRate)
	assert.Equal(map[ServiceSignature]float64{
		{"service1", "none"}: 0.3,
		{"service2", "none"}: 0.7,
		{}:                   0.2,
	}, rateByService)

	delete(rates, sig1)

	rateByService = cat.ratesByService(rates, totalRate)
	assert.Equal(map[ServiceSignature]float64{
		{"service2", "none"}: 0.7,
		{}:                   0.2,
	}, rateByService)

	delete(rates, sig2)

	rateByService = cat.ratesByService(rates, totalRate)
	assert.Equal(map[ServiceSignature]float64{
		{}: 0.2,
	}, rateByService)
}
