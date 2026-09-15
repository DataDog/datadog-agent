// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package report

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

func TestReportFDB(t *testing.T) {
	collectTime := time.Unix(1415792726, 0)
	config := &checkconfig.CheckConfig{
		DeviceID:           "default:1.2.3.4",
		Namespace:          "default",
		ResolvedSubnetName: "10.0.0.0/24",
	}

	t.Run("entries are timestamped by the payload", func(t *testing.T) {
		sender := mocksender.NewMockSender(t, "testID")
		sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
		ms := &MetricSender{sender: sender}

		ms.ReportFDB(config, collectTime, []metadata.FDBEntryMetadata{{
			DeviceID:       config.DeviceID,
			FDBID:          1,
			MacAddress:     "00:09:0f:09:0a:09",
			InterfaceIndex: 1000014,
		}})

		var got metadata.NetworkDevicesMetadata
		payload := sender.Calls[0].Arguments.Get(0).([]byte)
		require.NoError(t, json.Unmarshal(payload, &got))
		require.Len(t, got.FDBEntries, 1)
		assert.Equal(t, "00:09:0f:09:0a:09", got.FDBEntries[0].MacAddress)
		assert.Equal(t, collectTime.Unix(), got.CollectTimestamp)
		assert.Equal(t, "network-devices-metadata", sender.Calls[0].Arguments.Get(1))

		var raw map[string]any
		require.NoError(t, json.Unmarshal(payload, &raw))
		assert.NotContains(t, raw, "fdb_status")
		entry := raw["fdb_entries"].([]any)[0].(map[string]any)
		assert.Len(t, entry, 4)
		assert.Contains(t, entry, "device_id")
		assert.Contains(t, entry, "fdb_id")
		assert.Contains(t, entry, "mac_address")
		assert.Contains(t, entry, "interface_index")
	})

	t.Run("empty observations send nothing", func(t *testing.T) {
		sender := mocksender.NewMockSender(t, "testID")
		ms := &MetricSender{sender: sender}

		ms.ReportFDB(config, collectTime, nil)

		sender.AssertNotCalled(t, "EventPlatformEvent", mock.Anything, mock.Anything)
	})
}
