// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"math"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCatalogRegression is a regression tests ensuring that there is no race
// occurring when registering entries in the catalog in parallel to obtaining
// the rates by service map.
func TestCatalogRegression(_ *testing.T) {
	cat := newServiceLookup(0)
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
			cat.ratesByService("", map[Signature]float64{
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
	cat := newServiceLookup(0)
	assert.NotNil(t, cat.items)
	assert.NotNil(t, cat.ll)
}

func TestServiceKeyCatalogRegister(t *testing.T) {
	cat := newServiceLookup(0)
	s := getTestPrioritySampler()

	_, root1 := getTestTraceWithService("service1", s)
	sig1 := cat.register(ServiceSignature{root1.Service, defaultEnv})
	catalogContains(t, cat, map[ServiceSignature]Signature{
		{"service1", "testEnv"}: sig1,
	})

	_, root2 := getTestTraceWithService("service2", s)
	sig2 := cat.register(ServiceSignature{root2.Service, defaultEnv})
	catalogContains(t, cat, map[ServiceSignature]Signature{
		{"service1", "testEnv"}: sig1,
		{"service2", "testEnv"}: sig2,
	})
}

func TestServiceKeyCatalogLRU(t *testing.T) {
	t.Run("size", func(t *testing.T) {
		cat := newServiceLookup(0)
		cat.maxEntries = 3
		_ = cat.register(ServiceSignature{"service1", "env1"})
		sig2 := cat.register(ServiceSignature{"service2", "env2"})
		sig3 := cat.register(ServiceSignature{"service3", "env3"})
		sig4 := cat.register(ServiceSignature{"service4", "env4"})
		catalogContains(t, cat, map[ServiceSignature]Signature{
			{"service2", "env2"}: sig2,
			{"service3", "env3"}: sig3,
			{"service4", "env4"}: sig4,
		})
		sig5 := cat.register(ServiceSignature{"service5", "env5"})
		catalogContains(t, cat, map[ServiceSignature]Signature{
			{"service3", "env3"}: sig3,
			{"service4", "env4"}: sig4,
			{"service5", "env5"}: sig5,
		})
	})

	t.Run("move", func(t *testing.T) {
		cat := newServiceLookup(0)
		cat.maxEntries = 3
		sig1 := cat.register(ServiceSignature{"service1", "env1"})
		_ = cat.register(ServiceSignature{"service2", "env2"})
		sig3 := cat.register(ServiceSignature{"service3", "env3"})
		cat.register(ServiceSignature{"service1", "env1"}) // sig1 is moved, so 2 will be out
		sig4 := cat.register(ServiceSignature{"service4", "env4"})
		catalogContains(t, cat, map[ServiceSignature]Signature{
			{"service1", "env1"}: sig1,
			{"service3", "env3"}: sig3,
			{"service4", "env4"}: sig4,
		})
	})
}

func catalogContains(t *testing.T, cat *serviceKeyCatalog, has map[ServiceSignature]Signature) {
	assert := assert.New(t)
	assert.Len(cat.items, len(has), "too many items in map")
	assert.Equal(len(has), cat.ll.Len(), "too many elements in list")
	for el := cat.ll.Back(); el != nil; el = el.Prev() {
		key := el.Value.(catalogEntry).key
		if le, ok := cat.items[key]; !ok {
			t.Fatalf("Foreign item in map: %s", key)
			return
		} else if le != el {
			t.Fatalf("List element in map incorrect for key %v", key)
			return
		}
		val := el.Value.(catalogEntry).sig
		want, ok := has[key]
		if !ok {
			t.Fatalf("Foreign item in list: %s", key)
			return
		}
		if val != want {
			t.Fatalf("Invalid value %v (!=%v) in list at key %v", val, want, key)
			return
		}
	}
}

func TestCatalogEnvMatchAgent(t *testing.T) {
	assert := assert.New(t)
	cat := newServiceLookup(0)

	sig1 := ServiceSignature{"service1", defaultEnv}
	cat.register(sig1)
	sig2 := ServiceSignature{"service2", defaultEnv}
	cat.register(sig2)

	rates := map[Signature]float64{
		sig1.Hash(): 0.3,
		sig2.Hash(): 0.7,
	}
	const totalRate = 0.2

	rateByService := cat.ratesByService(defaultEnv, rates, totalRate)
	assert.Equal(map[ServiceSignature]float64{
		{"service1", defaultEnv}: 0.3,
		{"service1", ""}:         0.3,
		{"service2", defaultEnv}: 0.7,
		{"service2", ""}:         0.7,
		{}:                       0.2,
	}, rateByService)
}

func TestServiceKeyCatalogRatesByService(t *testing.T) {
	assert := assert.New(t)

	cat := newServiceLookup(0)
	s := getTestPrioritySampler()

	_, root1 := getTestTraceWithService("service1", s)
	sig1 := cat.register(ServiceSignature{root1.Service, defaultEnv})
	_, root2 := getTestTraceWithService("service2", s)
	sig2 := cat.register(ServiceSignature{root2.Service, defaultEnv})

	rates := map[Signature]float64{
		sig1: 0.3,
		sig2: 0.7,
	}
	const totalRate = 0.2

	rateByService := cat.ratesByService("", rates, totalRate)
	assert.Equal(map[ServiceSignature]float64{
		{"service1", "testEnv"}: 0.3,
		{"service2", "testEnv"}: 0.7,
		{}:                      0.2,
	}, rateByService)

	delete(rates, sig1)

	rateByService = cat.ratesByService("", rates, totalRate)
	assert.Equal(map[ServiceSignature]float64{
		{"service2", "testEnv"}: 0.7,
		{}:                      0.2,
	}, rateByService)

	delete(rates, sig2)

	rateByService = cat.ratesByService("", rates, totalRate)
	assert.Equal(map[ServiceSignature]float64{
		{}: 0.2,
	}, rateByService)
}

func BenchmarkServiceKeyCatalog(b *testing.B) {
	b.ReportAllocs()

	b.Run("new", func(b *testing.B) {
		x := 1
		cat := newServiceLookup(0)
		cat.maxEntries = math.MaxInt16
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			x *= 2
			ss := ServiceSignature{Name: strconv.Itoa(x), Env: strconv.Itoa(x)}
			cat.register(ss)
		}
	})

	b.Run("same", func(b *testing.B) {
		cat := newServiceLookup(0)
		cat.maxEntries = math.MaxInt16
		ss := ServiceSignature{Name: "sql-db", Env: "staging"}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cat.register(ss)
		}
	})
}
