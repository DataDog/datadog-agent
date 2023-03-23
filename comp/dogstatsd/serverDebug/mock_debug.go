// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverDebug

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"go.uber.org/atomic"
)

type mockServerDebug struct {
	sync.Mutex
	enabled *atomic.Bool
}

func newMockServerDebug(deps dependencies) Component {
	return &mockServerDebug{enabled: atomic.NewBool(false)}
}

func (d *mockServerDebug) StoreMetricStats(sample metrics.MetricSample) {
}

func (d *mockServerDebug) SetMetricStatsEnabled(enable bool) {
	d.enabled.Store(enable)
}

func (d *mockServerDebug) GetJSONDebugStats() ([]byte, error) {
	return []byte{}, nil
}

func (d *mockServerDebug) IsDebugEnabled() bool {
	return d.enabled.Load()
}
