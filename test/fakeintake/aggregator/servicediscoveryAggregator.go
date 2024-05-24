// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package aggregator

import (
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"time"
)

// ServiceDiscoveryPayload is a payload type for the service_discovery check
type ServiceDiscoveryPayload struct {
	collectedTime time.Time

	RequestType string `json:"request_type"`
	APIVersion  string `json:"api_version"`
	Payload     struct {
		NamingSchemaVersion string `json:"naming_schema_version"`
		ServiceName         string `json:"service_name"`
		HostName            string `json:"host_name"`
		Env                 string `json:"env"`
		ServiceLanguage     string `json:"service_language"`
		ServiceType         string `json:"service_type"`
		StartTime           int64  `json:"start_time"`
		LastSeen            int64  `json:"last_seen"`
		APMInstrumentation  string `json:"apm_instrumentation"`
		ServiceNameSource   string `json:"service_name_source"`
	} `json:"payload"`
}

func (s *ServiceDiscoveryPayload) name() string {
	return s.RequestType
}

func (s *ServiceDiscoveryPayload) GetTags() []string {
	return nil
}

func (s *ServiceDiscoveryPayload) GetCollectedTime() time.Time {
	return s.collectedTime
}

// ParseServiceDiscoveryPayload parses an api.Payload into a list of ServiceDiscoveryPayload
func ParseServiceDiscoveryPayload(payload api.Payload) ([]*ServiceDiscoveryPayload, error) {
	enflated, err := enflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, fmt.Errorf("could not enflate payload: %w", err)
	}
	var payloads []*ServiceDiscoveryPayload
	err = json.Unmarshal(enflated, &payloads)
	if err != nil {
		return nil, err
	}
	for _, p := range payloads {
		p.collectedTime = payload.Timestamp
	}
	return payloads, nil
}

// ServiceDiscoveryAggregator is an Aggregator for ServiceDiscoveryPayload
type ServiceDiscoveryAggregator struct {
	Aggregator[*ServiceDiscoveryPayload]
}

// NewServiceDiscoveryAggregator returns a new ServiceDiscoveryAggregator
func NewServiceDiscoveryAggregator() ServiceDiscoveryAggregator {
	return ServiceDiscoveryAggregator{
		Aggregator: newAggregator(ParseServiceDiscoveryPayload),
	}
}
