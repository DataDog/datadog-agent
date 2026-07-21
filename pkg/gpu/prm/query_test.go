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
	"github.com/stretchr/testify/require"
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
	mockDevice := setupMockDevice(func(device *testDevice) {
		device.readWritePRM = func(buffer *nvml.PRMTLV_v1) error {
			port := int(binary.BigEndian.Uint32(buffer.InData[20:24]) >> 16)
			response := makePLRResponseBytes(uint64(port * 100))
			copy(buffer.InData[:], response)
			return nil
		}
	})

	counters, err := QueryPortCounters(mockDevice, PPCNTGroupPLR, 2)
	require.NoError(t, err)
	require.Len(t, counters, len(PLRCounterFields))
	require.Equal(t, uint64(200), counters[PLRCounterFields[0]])
}

func TestQueryPortCountersError(t *testing.T) {
	mockDevice := setupMockDevice(func(device *testDevice) {
		device.readWritePRM = func(_ *nvml.PRMTLV_v1) error {
			return nvml.ERROR_UNKNOWN
		}
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

func setupMockDevice(customize func(device *testDevice)) Device {
	device := &testDevice{arch: nvml.DEVICE_ARCH_BLACKWELL}
	if customize != nil {
		customize(device)
	}
	return device
}
