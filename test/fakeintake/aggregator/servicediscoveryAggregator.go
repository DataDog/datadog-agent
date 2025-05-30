// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package aggregator

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// TracerMetadata is part of the payload for the service_discovery check
type TracerMetadata struct {
	SchemaVersion  uint8  `json:"schema_version"`
	RuntimeID      string `json:"runtime_id"`
	TracerLanguage string `json:"tracer_language"`
	ServiceName    string `json:"service_name"`
}

// ServiceDiscoveryPayload is a payload type for the service_discovery check
type ServiceDiscoveryPayload struct {
	collectedTime time.Time

	RequestType string `json:"request_type"`
	APIVersion  string `json:"api_version"`
	Payload     struct {
		NamingSchemaVersion  string           `json:"naming_schema_version"`
		GeneratedServiceName string           `json:"generated_service_name"`
		TracerMetadata       []TracerMetadata `json:"tracer_metadata"`
		DDService            string           `json:"dd_service,omitempty"`
		HostName             string           `json:"host_name"`
		Env                  string           `json:"env"`
		ServiceLanguage      string           `json:"service_language"`
		ServiceType          string           `json:"service_type"`
		StartTime            int64            `json:"start_time"`
		LastSeen             int64            `json:"last_seen"`
		APMInstrumentation   string           `json:"apm_instrumentation"`
		ServiceNameSource    string           `json:"service_name_source,omitempty"`
		RSSMemory            uint64           `json:"rss_memory"`
		CPUCores             float64          `json:"cpu_cores"`
		ContainerID          string           `json:"container_id"`
	} `json:"payload"`
}

func (s *ServiceDiscoveryPayload) name() string {
	return s.RequestType
}

// GetTags is not implemented.
func (s *ServiceDiscoveryPayload) GetTags() []string {
	return nil
}

// GetCollectedTime returns the time that the payload was received by the fake
// intake.
func (s *ServiceDiscoveryPayload) GetCollectedTime() time.Time {
	return s.collectedTime
}

// ParseServiceDiscoveryPayload parses an api.Payload into a list of
// ServiceDiscoveryPayload.
func ParseServiceDiscoveryPayload(payload api.Payload) ([]*ServiceDiscoveryPayload, error) {
	inflated, err := inflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, fmt.Errorf("could not inflate payload: %w", err)
	}
	var payloads []*ServiceDiscoveryPayload
	err = json.Unmarshal(inflated, &payloads)
	if err != nil {
		return nil, err
	}
	for _, p := range payloads {
		p.collectedTime = payload.Timestamp
	}
	return payloads, nil
}

// ServiceDiscoveryAggregator is an Aggregator for ServiceDiscoveryPayload.
type ServiceDiscoveryAggregator struct {
	Aggregator[*ServiceDiscoveryPayload]
}

// NewServiceDiscoveryAggregator returns a new ServiceDiscoveryAggregator.
func NewServiceDiscoveryAggregator() ServiceDiscoveryAggregator {
	return ServiceDiscoveryAggregator{
		Aggregator: newAggregator(ParseServiceDiscoveryPayload),
	}
}
