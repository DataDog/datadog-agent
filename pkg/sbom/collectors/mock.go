// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package collectors holds collectors related files
package collectors

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	"github.com/stretchr/testify/mock"
)

// MockCollector uses testify's mock to simulate a Collector
type MockCollector struct {
	mock.Mock
}

// NewMockCollector creates an instance of MockCollector
func NewMockCollector() *MockCollector {
	return &MockCollector{}
}

// Type returns the scan type of the collector
func (m *MockCollector) Type() ScanType {
	args := m.Called()
	return args.Get(0).(ScanType)
}

// CleanCache cleans the collector cache
func (m *MockCollector) CleanCache() error {
	args := m.Called()
	return args.Error(0)
}

// Init initializes the collector
func (m *MockCollector) Init(cfg config.Component, opt optional.Option[workloadmeta.Component]) error {
	args := m.Called(cfg, opt)
	return args.Error(0)
}

// Scan performs a scan
func (m *MockCollector) Scan(ctx context.Context, request sbom.ScanRequest) sbom.ScanResult {
	args := m.Called(ctx, request)
	return args.Get(0).(sbom.ScanResult)
}

// Channel returns the channel to send scan results
func (m *MockCollector) Channel() chan sbom.ScanResult {
	args := m.Called()
	return args.Get(0).(chan sbom.ScanResult)
}

// Options returns the collector options
func (m *MockCollector) Options() sbom.ScanOptions {
	args := m.Called()
	return args.Get(0).(sbom.ScanOptions)
}

// Shutdown shuts down the collector
func (m *MockCollector) Shutdown() {
	m.Called()
}

// Ensure MockCollector implements Collector
var _ Collector = (*MockCollector)(nil)
