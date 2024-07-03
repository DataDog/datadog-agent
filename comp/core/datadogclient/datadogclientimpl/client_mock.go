// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package datadogclientimpl

import (
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/datadogclient"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
	"gopkg.in/zorkian/go-datadog-api.v2"
)

type testDependencies struct {
	fx.In
	Lyfecycle fx.Lifecycle
}

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock),
	)
}

type mockDatadogClient struct {
	mux               sync.RWMutex
	queryMetricsFunc  func(from, to int64, query string) ([]datadog.Series, error)
	getRateLimitsFunc func() map[string]datadog.RateLimit
}

func (d *mockDatadogClient) QueryMetrics(from, to int64, query string) ([]datadog.Series, error) {
	d.mux.RLock()
	defer d.mux.RUnlock()
	if d.queryMetricsFunc != nil {
		return d.queryMetricsFunc(from, to, query)
	}
	return nil, nil
}

func (d *mockDatadogClient) GetRateLimitStats() map[string]datadog.RateLimit {
	d.mux.RLock()
	defer d.mux.RUnlock()
	if d.getRateLimitsFunc != nil {
		return d.getRateLimitsFunc()
	}
	return nil
}

func (d *mockDatadogClient) SetQueryMetricsFunc(queryMetricsFunc func(from, to int64, query string) ([]datadog.Series, error)) {
	d.mux.Lock()
	defer d.mux.Unlock()
	d.queryMetricsFunc = queryMetricsFunc
}

func (d *mockDatadogClient) SetGetRateLimitsFunc(getRateLimitsFunc func() map[string]datadog.RateLimit) {
	d.mux.Lock()
	defer d.mux.Unlock()
	d.getRateLimitsFunc = getRateLimitsFunc
}

// NewMock returns a new mock datadogclient component
func newMock(deps testDependencies) datadogclient.MockComponent {
	m := &mockDatadogClient{}
	return m
}
