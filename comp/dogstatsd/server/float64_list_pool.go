// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

// float64ListPool wraps the sync.Pool class for []float64 type.
// It avoids allocating a new slice for each packet received.
//
// Caution: as objects get reused, data in the slice will change when the
// object is reused. You need to hold on to the object until you extracted all
// the information needed.
type float64ListPool struct {
	pool sync.Pool
	// telemetry
	tlmEnabled            bool
	tlmFloat64ListPoolGet telemetry.Counter
	tlmFloat64ListPoolPut telemetry.Counter
	tlmFloat64ListPool    telemetry.Gauge
}

// newFloat64ListPool creates a new pool with a specified buffer size
func newFloat64ListPool(telemetrycomp telemetry.Component) *float64ListPool {
	return &float64ListPool{
		pool: sync.Pool{
			New: func() interface{} {
				return []float64{}
			},
		},
		// telemetry
		tlmEnabled: utils.IsTelemetryEnabled(config.Datadog()),
		tlmFloat64ListPoolGet: telemetrycomp.NewCounter("dogstatsd", "float64_list_pool_get",
			nil, "Count of get done in the float64_list  pool"),
		tlmFloat64ListPoolPut: telemetrycomp.NewCounter("dogstatsd", "float64_list_pool_put",
			nil, "Count of put done in the float64_list  pool"),
		tlmFloat64ListPool: telemetrycomp.NewGauge("dogstatsd", "float64_list_pool",
			nil, "Usage of the float64_list pool in dogstatsd"),
	}
}

// Get gets a slice of floats ready to use.
func (f *float64ListPool) get() []float64 {
	if f.tlmEnabled {
		f.tlmFloat64ListPoolGet.Inc()
		f.tlmFloat64ListPool.Inc()
	}
	return f.pool.Get().([]float64)
}

// Put resets the slice of floats and puts it back in the pool.
func (f *float64ListPool) put(list []float64) {
	if f.tlmEnabled {
		f.tlmFloat64ListPoolPut.Inc()
		f.tlmFloat64ListPool.Dec()
	}
	// we reset the slice's length but keep the allocated buffer
	list = list[:0]
	f.pool.Put(list)
}
