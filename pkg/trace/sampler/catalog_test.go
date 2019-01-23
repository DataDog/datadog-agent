package sampler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
