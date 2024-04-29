// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package serverdebugimpl

import (
	"sync"

	serverdebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"go.uber.org/atomic"
	"go.uber.org/fx"
)

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMockServerDebug))
}

type mockServerDebug struct {
	sync.Mutex
	enabled *atomic.Bool
}

func newMockServerDebug() serverdebug.Component {
	return &mockServerDebug{enabled: atomic.NewBool(false)}
}

func (d *mockServerDebug) StoreMetricStats(_ metrics.MetricSample) {
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
