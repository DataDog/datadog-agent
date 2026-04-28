// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package prm

import (
	"encoding/binary"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

func TestPackUnpackTLVRoundTrip(t *testing.T) {
	expected := make(map[string]uint64, len(PLRCounterFields))
	payload := make([]byte, ppcntSizeBytes)
	binary.BigEndian.PutUint32(payload[0:4], PPCNTGroupPLR)

	offset := 8
	for i, field := range PLRCounterFields {
		value := uint64(i+1)<<32 | uint64(100+i)
		expected[field] = value
		binary.BigEndian.PutUint32(payload[offset:offset+4], uint32(value>>32))
		offset += 4
		binary.BigEndian.PutUint32(payload[offset:offset+4], uint32(value))
		offset += 4
	}

	packet := packTLV(ppcntRegID, ppcntSizeBytes, payload)
	metrics, err := UnpackTLV(packet)
	require.NoError(t, err)
	require.Equal(t, expected, metrics)
}

func TestQueryPortCounters(t *testing.T) {
	mockDevice := setupMockDevice(t, func(device *mock.Device) *mock.Device {
		testutil.WithMockAllDeviceFunctions()(device)
		device.ReadWritePRM_v1Func = func(buffer *nvml.PRMTLV_v1) nvml.Return {
			port := int(binary.BigEndian.Uint32(buffer.InData[20:24]) >> 16)
			response := makePLRResponseBytes(uint64(port * 100))
			copy(buffer.InData[:], response)
			return nvml.SUCCESS
		}
		return device
	})

	counters, err := QueryPortCounters(mockDevice, PPCNTGroupPLR, 2)
	require.NoError(t, err)
	require.Len(t, counters, len(PLRCounterFields))
	require.Equal(t, uint64(200), counters[PLRCounterFields[0]])
}

func TestQueryPortCountersError(t *testing.T) {
	mockDevice := setupMockDevice(t, func(device *mock.Device) *mock.Device {
		testutil.WithMockAllDeviceFunctions()(device)
		device.ReadWritePRM_v1Func = func(_ *nvml.PRMTLV_v1) nvml.Return {
			return nvml.ERROR_UNKNOWN
		}
		return device
	})

	_, err := QueryPortCounters(mockDevice, PPCNTGroupPLR, 1)
	require.Error(t, err)
	require.ErrorContains(t, err, "issue raw PRM query")
}

func makePLRResponseBytes(seed uint64) []byte {
	payload := make([]byte, ppcntSizeBytes)
	binary.BigEndian.PutUint32(payload[0:4], PPCNTGroupPLR)
	offset := 8
	for i := range PLRCounterFields {
		value := seed + uint64(i)
		binary.BigEndian.PutUint32(payload[offset:offset+4], uint32(value>>32))
		offset += 4
		binary.BigEndian.PutUint32(payload[offset:offset+4], uint32(value))
		offset += 4
	}
	return packTLV(ppcntRegID, ppcntSizeBytes, payload)
}

func setupMockDevice(t *testing.T, customize func(device *mock.Device) *mock.Device) ddnvml.Device {
	t.Helper()

	nvmlMock := testutil.GetBasicNvmlMockWithOptions(
		testutil.WithMIGDisabled(),
		testutil.WithDeviceCount(1),
	)
	device := testutil.GetDeviceMock(0, testutil.WithMockAllDeviceFunctions())
	if customize != nil {
		device = customize(device)
	}

	nvmlMock.DeviceGetHandleByIndexFunc = func(index int) (nvml.Device, nvml.Return) {
		if index == 0 {
			return device, nvml.SUCCESS
		}
		return nil, nvml.ERROR_INVALID_ARGUMENT
	}

	ddnvml.WithMockNVML(t, nvmlMock)
	deviceCache := ddnvml.NewDeviceCache()
	devices, err := deviceCache.AllPhysicalDevices()
	require.NoError(t, err)
	require.Len(t, devices, 1)
	return devices[0]
}

