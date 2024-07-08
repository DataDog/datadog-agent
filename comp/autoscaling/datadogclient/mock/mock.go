// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package mock provides mock methods
package mock

import (
	"sync"
	"testing"

	datadogclient "github.com/DataDog/datadog-agent/comp/autoscaling/datadogclient/def"
	"gopkg.in/zorkian/go-datadog-api.v2"
)

type mockDatadogClient struct {
	mux               sync.RWMutex
	queryMetricsFunc  func(from, to int64, query string) ([]datadog.Series, error)
	getRateLimitsFunc func() map[string]datadog.RateLimit
}

var _ datadogclient.Component = (*mockDatadogClient)(nil)

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
func NewMock(*testing.T) datadogclient.MockComponent {
	m := &mockDatadogClient{}
	return m
}
