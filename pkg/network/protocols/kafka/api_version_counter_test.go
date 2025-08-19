// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package kafka

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
)

func TestAPIVersionCounter(t *testing.T) {
	tests := []struct {
		name                 string
		minVersion           uint16
		maxVersion           uint16
		tx1                  *KafkaTransaction
		tx2                  *KafkaTransaction
		expectedUnsupported  int64
		expectedHitsVersions map[uint16]int64
	}{
		{
			name:       "Sanity",
			minVersion: 1,
			maxVersion: 3,
			tx1:        &KafkaTransaction{Request_api_version: 1, Records_count: 5},
			tx2:        &KafkaTransaction{Request_api_version: 3, Records_count: 10},
			expectedHitsVersions: map[uint16]int64{
				1: 5,
				3: 10,
			},
		},
		{
			name:                "One unsupported version",
			minVersion:          1,
			maxVersion:          3,
			tx1:                 &KafkaTransaction{Request_api_version: 0, Records_count: 7},
			tx2:                 &KafkaTransaction{Request_api_version: 2, Records_count: 5},
			expectedUnsupported: 7,
			expectedHitsVersions: map[uint16]int64{
				2: 5,
			},
		},
		{
			name:                "Two unsupported versions",
			minVersion:          1,
			maxVersion:          3,
			tx1:                 &KafkaTransaction{Request_api_version: 0, Records_count: 10},
			tx2:                 &KafkaTransaction{Request_api_version: 5, Records_count: 10},
			expectedUnsupported: 20,
		},
		{
			name:       "Valid versions within range",
			minVersion: 1,
			maxVersion: 3,
			tx1:        &KafkaTransaction{Request_api_version: 2, Records_count: 8},
			tx2:        &KafkaTransaction{Request_api_version: 3, Records_count: 12},
			expectedHitsVersions: map[uint16]int64{
				2: 8,
				3: 12,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The name have to be unique, otherwise the metric will aggregate the values from different tests
			metricGroup := telemetry.NewMetricGroup(tt.name)
			counter := newAPIVersionCounter(metricGroup, "kafka_requests", tt.minVersion, tt.maxVersion, "test_tag")

			counter.Add(tt.tx1)
			counter.Add(tt.tx2)

			assert.Equal(t, tt.expectedUnsupported, counter.hitsUnsupportedVersion.Get(), "Unexpected unsupported version count")

			for version, expectedCount := range tt.expectedHitsVersions {
				assert.Equal(t, expectedCount, counter.hitsVersions[version-tt.minVersion].Get(), "Unexpected hit count for API version %d", version)
			}
		})
	}
}
