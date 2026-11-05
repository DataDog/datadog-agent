// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the demultiplexer component
package mock

import (
	"sync"
	"testing"

	demultiplexer "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/def"
	demultiplexerimpl "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/impl"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	metricscompressionmock "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-mock"
)

type mock struct {
	demultiplexer.Mock
	stopOnce sync.Once
}

func (m *mock) Stop() {
	m.stopOnce.Do(m.Mock.Stop)
}

// New returns a mock demultiplexer that is stopped when the test finishes.
func New(t testing.TB) demultiplexer.Mock {
	t.Helper()
	hostname, _ := hostnamemock.NewMock("my-hostname")
	instance := &mock{Mock: demultiplexerimpl.NewMock(logmock.New(t), hostname)}
	t.Cleanup(instance.Stop)
	return instance
}

// NewFakeSamplerMock returns a fake sampler demultiplexer that is stopped when the test finishes.
func NewFakeSamplerMock(t testing.TB) demultiplexer.FakeSamplerMock {
	t.Helper()
	hostname, _ := hostnamemock.NewMock("my-hostname")
	instance := demultiplexerimpl.NewFakeSamplerMock(logmock.New(t), hostname, metricscompressionmock.NewMockCompressor())
	t.Cleanup(instance.Stop)
	return instance
}
