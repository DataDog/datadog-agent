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
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/fdb"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

func TestReportFDBSuccessAndTruncated(t *testing.T) {
	collectTime := time.Unix(1415792726, 0)
	config := &checkconfig.CheckConfig{
		DeviceID:           "default:1.2.3.4",
		Namespace:          "default",
		ResolvedSubnetName: "10.0.0.0/24",
	}

	t.Run("success sends rows and status", func(t *testing.T) {
		sender := mocksender.NewMockSender(t, "testID")
		sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
		ms := &MetricSender{sender: sender}

		ms.ReportFDB(config, collectTime, fdb.Result{
			Status: metadata.FDBStatusMetadata{
				DeviceID: config.DeviceID,
				Status:   metadata.FDBCollectStatusSuccess,
				Source:   metadata.FDBSourceQBridge,
				RowCount: 1,
			},
			Entries: []metadata.FDBEntryMetadata{{
				DeviceID:       config.DeviceID,
				FDBID:          "1",
				MacAddress:     "00:09:0f:09:0a:09",
				BridgePort:     105,
				InterfaceIndex: 1000014,
				InterfaceID:    config.DeviceID + ":1000014",
				Source:         metadata.FDBSourceQBridge,
			}},
		})

		var got metadata.NetworkDevicesMetadata
		payload := sender.Calls[0].Arguments.Get(0).([]byte)
		require.NoError(t, json.Unmarshal(payload, &got))
		require.NotNil(t, got.FDBStatus)
		assert.Equal(t, metadata.FDBCollectStatusSuccess, got.FDBStatus.Status)
		require.Len(t, got.FDBEntries, 1)
		assert.Equal(t, "00:09:0f:09:0a:09", got.FDBEntries[0].MacAddress)
		assert.Equal(t, "network-devices-metadata", sender.Calls[0].Arguments.Get(1))
	})

	t.Run("truncated sends status only", func(t *testing.T) {
		sender := mocksender.NewMockSender(t, "testID")
		sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
		ms := &MetricSender{sender: sender}

		ms.ReportFDB(config, collectTime, fdb.Result{
			Status: metadata.FDBStatusMetadata{
				DeviceID: config.DeviceID,
				Status:   metadata.FDBCollectStatusTruncated,
				Reason:   "max_entries",
			},
		})

		var got metadata.NetworkDevicesMetadata
		require.NoError(t, json.Unmarshal(sender.Calls[0].Arguments.Get(0).([]byte), &got))
		require.NotNil(t, got.FDBStatus)
		assert.Equal(t, metadata.FDBCollectStatusTruncated, got.FDBStatus.Status)
		assert.Empty(t, got.FDBEntries)
	})
}
