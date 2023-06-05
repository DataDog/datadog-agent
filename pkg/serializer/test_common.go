// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package serializer

import (
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

// MockSerializer is a mock for the MetricSerializer
type MockSerializer struct {
	mock.Mock
}

// SendEvents serializes a list of event and sends the payload to the forwarder
func (s *MockSerializer) SendEvents(events metrics.Events) error {
	return s.Called(events).Error(0)
}

// SendServiceChecks serializes a list of serviceChecks and sends the payload to the forwarder
func (s *MockSerializer) SendServiceChecks(serviceChecks metrics.ServiceChecks) error {
	return s.Called(serviceChecks).Error(0)
}

// SendIterableSeries serializes a list of Serie and sends the payload to the forwarder
func (s *MockSerializer) SendIterableSeries(serieSource metrics.SerieSource) error {
	return s.Called(serieSource).Error(0)
}

// AreSeriesEnabled returns whether series are enabled for serialization
func (s *MockSerializer) AreSeriesEnabled() bool { return true }

// SendSketch serializes a list of SketSeriesList and sends the payload to the forwarder
func (s *MockSerializer) SendSketch(sketches metrics.SketchesSource) error {
	return s.Called(sketches).Error(0)
}

// AreSeriesEnabled returns whether sketches are enabled for serialization
func (s *MockSerializer) AreSketchesEnabled() bool { return true }

// SendMetadata serializes a metadata payload and sends it to the forwarder
func (s *MockSerializer) SendMetadata(m marshaler.JSONMarshaler) error {
	return s.Called(m).Error(0)
}

// SendHostMetadata serializes a host metadata payload and sends it to the forwarder
func (s *MockSerializer) SendHostMetadata(m marshaler.JSONMarshaler) error {
	return s.Called(m).Error(0)
}

// SendAgentchecksMetadata serializes a metadata payload and sends it to the forwarder
func (s *MockSerializer) SendAgentchecksMetadata(m marshaler.JSONMarshaler) error {
	return s.Called(m).Error(0)
}

// SendProcessesMetadata serializes a legacy process metadata payload and sends it to the forwarder.
func (s *MockSerializer) SendProcessesMetadata(data interface{}) error {
	return s.Called(data).Error(0)
}

// SendOrchestratorMetadata serializes & sends orchestrator metadata payloads
func (s *MockSerializer) SendOrchestratorMetadata(msgs []ProcessMessageBody, hostName, clusterID string, payloadType int) error {
	return s.Called(msgs, hostName, clusterID, payloadType).Error(0)
}

// SendOrchestratorManifests serializes & send orchestrator manifest payloads
func (s *MockSerializer) SendOrchestratorManifests(msgs []ProcessMessageBody, hostName, clusterID string) error {
	return s.Called(msgs, hostName, clusterID).Error(0)
}
