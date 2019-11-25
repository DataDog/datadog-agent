// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package testutil

import (
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
)

// MockEngine mocks a sampler engine
type MockEngine struct {
	wantSampled bool
	wantRate    float64
}

// NewMockEngine returns a MockEngine for tests
func NewMockEngine(wantSampled bool, wantRate float64) *MockEngine {
	return &MockEngine{wantSampled: wantSampled, wantRate: wantRate}
}

// Sample returns a constant rate
func (e *MockEngine) Sample(_ pb.Trace, _ *pb.Span, _ string) (bool, float64) {
	return e.wantSampled, e.wantRate
}

// Run mocks Engine.Run()
func (e *MockEngine) Run() {
	return
}

// Stop mocks Engine.Stop()
func (e *MockEngine) Stop() {
	return
}

// GetState mocks Engine.GetState()
func (e *MockEngine) GetState() interface{} {
	return nil
}

// GetType mocks Engine.GetType()
func (e *MockEngine) GetType() sampler.EngineType {
	return sampler.NormalScoreEngineType
}
