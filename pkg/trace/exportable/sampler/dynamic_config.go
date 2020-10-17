// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package sampler

import (
	"sync"
)

// DynamicConfig contains configuration items which may change
// dynamically over time.
type DynamicConfig struct {
	// RateByService contains the rate for each service/env tuple,
	// used in priority sampling by client libs.
	RateByService RateByService
}

// NewDynamicConfig creates a new dynamic config object which maps service signatures
// to their corresponding sampling rates. Each service will have a default assigned
// matching the service rate of the specified env.
func NewDynamicConfig(env string) *DynamicConfig {
	return &DynamicConfig{RateByService: RateByService{defaultEnv: env}}
}

// RateByService stores the sampling rate per service. It is thread-safe, so
// one can read/write on it concurrently, using getters and setters.
type RateByService struct {
	defaultEnv string // env. to use for service defaults

	mu    sync.RWMutex // guards rates
	rates map[string]float64
}

// SetAll the sampling rate for all services. If a service/env is not
// in the map, then the entry is removed.
func (rbs *RateByService) SetAll(rates map[ServiceSignature]float64) {
	rbs.mu.Lock()
	defer rbs.mu.Unlock()

	if rbs.rates == nil {
		rbs.rates = make(map[string]float64, len(rates))
	}
	for k := range rbs.rates {
		delete(rbs.rates, k)
	}
	for k, v := range rates {
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		rbs.rates[k.String()] = v
		if k.Env == rbs.defaultEnv {
			// if this is the default env, then this is also the
			// service's default rate unbound to any env.
			rbs.rates[ServiceSignature{Name: k.Name}.String()] = v
		}
	}
}

// GetAll returns all sampling rates for all services.
func (rbs *RateByService) GetAll() map[string]float64 {
	rbs.mu.RLock()
	defer rbs.mu.RUnlock()

	ret := make(map[string]float64, len(rbs.rates))
	for k, v := range rbs.rates {
		ret[k] = v
	}

	return ret
}
