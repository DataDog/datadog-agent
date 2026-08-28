// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package report

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	snmpmetadata "github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/metadata"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

func cdpStoreWithRemoteDevice(sysName string, deviceID string) *snmpmetadata.Store {
	const index = "7.1"

	store := snmpmetadata.NewMetadataStore()
	store.AddColumnValue("cdp_remote.interface_id", index, valuestore.ResultValue{Value: "GigabitEthernet0/1"})
	store.AddColumnValue("cdp_remote.device_name", index, valuestore.ResultValue{Value: sysName})
	store.AddColumnValue("cdp_remote.device_id", index, valuestore.ResultValue{Value: deviceID})

	return store
}

func TestBuildNetworkTopologyMetadataWithCDP_remoteDeviceName(t *testing.T) {
	tests := []struct {
		name         string
		sysName      string
		deviceID     string
		expectedName string
	}{
		{
			name:         "cdpCacheSysName is used when the device emits the optional SysName TLV",
			sysName:      "spine-01.example.internal",
			deviceID:     "FDO1234ABCD",
			expectedName: "spine-01.example.internal",
		},
		{
			name:         "cdpCacheDeviceId is used when cdpCacheSysName is absent",
			sysName:      "",
			deviceID:     "NYC-36FL-AP02",
			expectedName: "NYC-36FL-AP02",
		},
		{
			name:         "remote device name stays empty when neither OID is populated",
			sysName:      "",
			deviceID:     "",
			expectedName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := cdpStoreWithRemoteDevice(tt.sysName, tt.deviceID)

			links := buildNetworkTopologyMetadataWithCDP("default:10.0.0.1", store, nil)

			require.Len(t, links, 1)
			assert.Equal(t, tt.expectedName, links[0].Remote.Device.Name)
			assert.Equal(t, tt.deviceID, links[0].Remote.Device.ID)
		})
	}
}
