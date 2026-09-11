// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package metadata

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/integrations"
)

func TestDiscoveredDeviceMetadataMarshalling(t *testing.T) {
	payload := NetworkDevicesMetadata{
		Namespace:        "default",
		Integration:      integrations.SNMP,
		CollectTimestamp: 1700000000,
		DiscoveredDevices: []DiscoveredDeviceMetadata{
			{
				AutodiscoveryID: "ad-1",
				RunID:           "run-1",
				IPAddress:       "10.0.0.4",
				Name:            "router-1",
				PingStatus:      "reachable",
				SNMPStatus:      "reachable",
				SNMPCredID:      "cred-a",
			},
		},
	}

	out, err := json.Marshal(payload)
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"namespace": "default",
		"integration": "snmp",
		"collect_timestamp": 1700000000,
		"discovered_devices": [{
			"autodiscovery_id": "ad-1",
			"run_id": "run-1",
			"ip_address": "10.0.0.4",
			"name": "router-1",
			"ping_status": "reachable",
			"snmp_status": "reachable",
			"snmp_cred_id": "cred-a"
		}]
	}`, string(out))
}

func TestAutodiscoveryRunMetadataMarshalling(t *testing.T) {
	payload := NetworkDevicesMetadata{
		Namespace:        "default",
		Integration:      integrations.SNMP,
		CollectTimestamp: 1700000000,
		AutodiscoveryRuns: []AutodiscoveryRunMetadata{
			{
				AutodiscoveryID:  "ad-1",
				RunID:            "run-1",
				Status:           AutodiscoveryRunCompleted,
				AddressesScanned: 65536,
				StartedAtMs:      1699000000000,
				FinishedAtMs:     1700000000000,
			},
		},
	}

	out, err := json.Marshal(payload)
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"namespace": "default",
		"integration": "snmp",
		"collect_timestamp": 1700000000,
		"autodiscovery_runs": [{
			"autodiscovery_id": "ad-1",
			"run_id": "run-1",
			"status": "completed",
			"addresses_scanned": 65536,
			"started_at_ms": 1699000000000,
			"finished_at_ms": 1700000000000
		}]
	}`, string(out))
}

func TestAutodiscoveryRunStatusesUseUnderscores(t *testing.T) {
	assert.Equal(t, AutodiscoveryRunStatus("in_progress"), AutodiscoveryRunInProgress)
	assert.Equal(t, AutodiscoveryRunStatus("completed"), AutodiscoveryRunCompleted)
	assert.Equal(t, AutodiscoveryRunStatus("failed"), AutodiscoveryRunFailed)
	assert.Equal(t, AutodiscoveryRunStatus("blocked"), AutodiscoveryRunBlocked)
}
