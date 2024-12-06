// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kafka

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
)

func TestTelemetry_Count(t *testing.T) {
	tests := []struct {
		name string
		tx1  *KafkaTransaction
		tx2  *KafkaTransaction
	}{
		{
			name: "Sanity",
			tx1: &KafkaTransaction{
				Request_api_key:     0,
				Request_api_version: 4,
				Records_count:       1,
			},
			tx2: &KafkaTransaction{
				Request_api_key:     1,
				Request_api_version: 7,
				Records_count:       4,
			},
		},
		{
			name: "One unsupported version",
			tx1: &KafkaTransaction{
				Request_api_key:     0,
				Request_api_version: 0,
				Records_count:       10,
			},
			tx2: &KafkaTransaction{
				Request_api_key:     1,
				Request_api_version: 7,
				Records_count:       10,
			},
		},
		{
			name: "Two unsupported version",
			tx1: &KafkaTransaction{
				Request_api_key:     0,
				Request_api_version: 0,
				Records_count:       10,
			},
			tx2: &KafkaTransaction{
				Request_api_key:     1,
				Request_api_version: 0,
				Records_count:       10,
			},
		},
		{
			name: "Unsupported api key",
			tx1: &KafkaTransaction{
				Request_api_key:     3,
				Request_api_version: 5,
				Records_count:       10,
			},
			tx2: &KafkaTransaction{
				Request_api_key:     1,
				Request_api_version: 8,
				Records_count:       10,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			telemetry.Clear()
			tel := NewTelemetry()
			tel.Count(tt.tx1)
			tel.Count(tt.tx2)
			verifyHitsCount(t, tel, tt.tx1)
			verifyHitsCount(t, tel, tt.tx2)
		})
	}
}

func verifyHitsCount(t *testing.T, telemetry *Telemetry, tx *KafkaTransaction) {
	if tx.Request_api_key == 0 {
		if tx.Request_api_version < minSupportedAPIVersion || tx.Request_api_version > maxSupportedAPIVersion {
			assert.Equal(t, telemetry.produceHits.hitsUnsupportedVersion.Get(), int64(tx.Records_count), "hitsUnsupportedVersion count is incorrect")
			return
		}
		assert.Equal(t, telemetry.produceHits.hitsVersions[tx.Request_api_version-1].Get(), int64(tx.Records_count), "produceHits count is incorrect")
	} else if tx.Request_api_key == 1 {
		if tx.Request_api_version < minSupportedAPIVersion || tx.Request_api_version > maxSupportedAPIVersion {
			assert.Equal(t, telemetry.fetchHits.hitsUnsupportedVersion.Get(), int64(tx.Records_count), "hitsUnsupportedVersion count is incorrect")
			return
		}
		assert.Equal(t, telemetry.fetchHits.hitsVersions[tx.Request_api_version-1].Get(), int64(tx.Records_count), "fetchHits count is incorrect")
	}
}
