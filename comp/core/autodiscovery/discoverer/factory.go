// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discoverer

import (
	discovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/discoverer/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
)

// NewFactory assembles the discovery engine with its probe implementation.
// Only products that include configuration discovery should import this package.
// Each config manager gets its own worker and retry state. probe must be non-nil.
func NewFactory(probe ConfigDiscoverer) discovery.Factory {
	return workerFactory{probe: probe}
}

type workerFactory struct {
	probe ConfigDiscoverer
}

func (f workerFactory) NewWorker(services discovery.ServiceLookup, onResult discovery.ResultCallback, telStore *telemetry.Store) discovery.Worker {
	return NewWorker(f.probe, services, onResult, Config{}, telStore)
}

var _ discovery.Worker = (*Worker)(nil)
