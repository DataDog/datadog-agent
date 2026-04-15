// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"encoding/binary"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"

	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

func TestCreatePPCNTTLVByteArray(t *testing.T) {
	packet := createPPCNTTLVByteArray(ppcntGroupPLR, 3)
	require.Len(t, packet, (opTLVLenDwords+regTLVHeaderLenDwords+endTLVLenDwords)*dwordSizeBytes+ppcntSizeBytes)

	require.Equal(t, makeTLVHeader(tlvTypeOp, opTLVLenDwords), binary.BigEndian.Uint32(packet[0:4]))
	require.Equal(t, makeOpMethodAndReg(ppcntRegID), binary.BigEndian.Uint32(packet[4:8]))
	require.Equal(t, uint32(0), binary.BigEndian.Uint32(packet[8:12]))
	require.Equal(t, uint32(0), binary.BigEndian.Uint32(packet[12:16]))

	require.Equal(t, makeTLVHeader(tlvTypeReg, uint32(ppcntSizeBytes/dwordSizeBytes+regTLVHeaderLenDwords)), binary.BigEndian.Uint32(packet[16:20]))
	require.Equal(t, uint32((ppcntGroupPLR&0x3F)|(3<<16)), binary.BigEndian.Uint32(packet[20:24]))
	require.Equal(t, makeTLVHeader(tlvTypeEnd, endTLVLenDwords), binary.BigEndian.Uint32(packet[len(packet)-4:]))
}

func TestUnpackTLV(t *testing.T) {
	expected := make(map[string]uint64, len(plrCounterFields))
	payload := make([]byte, ppcntSizeBytes)
	binary.BigEndian.PutUint32(payload[0:4], ppcntGroupPLR)

	offset := 8
	for i, field := range plrCounterFields {
		value := uint64(i+1)<<32 | uint64(100+i)
		expected[field] = value
		binary.BigEndian.PutUint32(payload[offset:offset+4], uint32(value>>32))
		offset += 4
		binary.BigEndian.PutUint32(payload[offset:offset+4], uint32(value))
		offset += 4
	}

	packet := packTLV(ppcntRegID, ppcntSizeBytes, payload)
	metrics, err := unpackTLV(packet)
	require.NoError(t, err)
	require.Equal(t, expected, metrics)
}

func TestNVLinkCollector(t *testing.T) {
	mockDevice := setupMockDeviceWithLibOpts(t, func(device *mock.Device) *mock.Device {
		testutil.WithMockAllDeviceFunctions()(device)
		device.GetFieldValuesFunc = func(values []nvml.FieldValue) nvml.Return {
			require.Len(t, values, 1)
			values[0].ValueType = uint32(nvml.VALUE_TYPE_UNSIGNED_INT)
			values[0].Value = [8]byte{2, 0, 0, 0, 0, 0, 0, 0}
			return nvml.SUCCESS
		}
		device.ReadWritePRM_v1Func = func(buffer *nvml.PRMTLV_v1) nvml.Return {
			port := int(binary.BigEndian.Uint32(buffer.InData[20:24]) >> 16)
			response := makePLRResponseBytes(uint64(port * 100))
			copy(buffer.InData[:], response)
			return nvml.SUCCESS
		}
		return device
	})

	collector, err := newNVLinkCollector(mockDevice, nil)
	require.NoError(t, err)
	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, len(plrCounterFields)*2)

	port1Count := 0
	port2Count := 0
	for _, metric := range metrics {
		switch {
		case containsTag(metric.Tags, "nvlink_port:1"):
			port1Count++
		case containsTag(metric.Tags, "nvlink_port:2"):
			port2Count++
		default:
			t.Fatalf("missing nvlink_port tag on metric %+v", metric)
		}
	}
	require.Equal(t, len(plrCounterFields), port1Count)
	require.Equal(t, len(plrCounterFields), port2Count)
}

func TestNVLinkCollectorPartialFailure(t *testing.T) {
	mockDevice := setupMockDeviceWithLibOpts(t, func(device *mock.Device) *mock.Device {
		testutil.WithMockAllDeviceFunctions()(device)
		device.GetFieldValuesFunc = func(values []nvml.FieldValue) nvml.Return {
			require.Len(t, values, 1)
			values[0].ValueType = uint32(nvml.VALUE_TYPE_UNSIGNED_INT)
			values[0].Value = [8]byte{2, 0, 0, 0, 0, 0, 0, 0}
			return nvml.SUCCESS
		}
		device.ReadWritePRM_v1Func = func(buffer *nvml.PRMTLV_v1) nvml.Return {
			port := int(binary.BigEndian.Uint32(buffer.InData[20:24]) >> 16)
			if port == 2 {
				return nvml.ERROR_NOT_SUPPORTED
			}
			response := makePLRResponseBytes(100)
			copy(buffer.InData[:], response)
			return nvml.SUCCESS
		}
		return device
	})

	collector, err := newNVLinkCollector(mockDevice, nil)
	require.NoError(t, err)
	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, len(plrCounterFields))
	for _, metric := range metrics {
		require.Contains(t, metric.Tags, "nvlink_port:1")
	}
}

func TestNVLinkCollectorKeepsPortOnTransientError(t *testing.T) {
	callCountByPort := map[int]int{}
	mockDevice := setupMockDeviceWithLibOpts(t, func(device *mock.Device) *mock.Device {
		testutil.WithMockAllDeviceFunctions()(device)
		device.GetFieldValuesFunc = func(values []nvml.FieldValue) nvml.Return {
			require.Len(t, values, 1)
			values[0].ValueType = uint32(nvml.VALUE_TYPE_UNSIGNED_INT)
			values[0].Value = [8]byte{2, 0, 0, 0, 0, 0, 0, 0}
			return nvml.SUCCESS
		}
		device.ReadWritePRM_v1Func = func(buffer *nvml.PRMTLV_v1) nvml.Return {
			port := int(binary.BigEndian.Uint32(buffer.InData[20:24]) >> 16)
			callCountByPort[port]++
			if port == 2 && callCountByPort[port] == 1 {
				return nvml.ERROR_UNKNOWN
			}
			response := makePLRResponseBytes(uint64(port * 100))
			copy(buffer.InData[:], response)
			return nvml.SUCCESS
		}
		return device
	})

	collector, err := newNVLinkCollector(mockDevice, nil)
	require.NoError(t, err)

	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, len(plrCounterFields)*2)
}

func TestNVLinkCollectorUnsupportedDevice(t *testing.T) {
	tests := []struct {
		name       string
		customize  func(*mock.Device) *mock.Device
	}{
		{
			name: "field API unsupported",
			customize: func(device *mock.Device) *mock.Device {
				testutil.WithMockAllDeviceFunctions()(device)
				device.GetFieldValuesFunc = func(_ []nvml.FieldValue) nvml.Return {
					return nvml.ERROR_NOT_SUPPORTED
				}
				return device
			},
		},
		{
			name: "no nvlink ports",
			customize: func(device *mock.Device) *mock.Device {
				testutil.WithMockAllDeviceFunctions()(device)
				device.GetFieldValuesFunc = func(values []nvml.FieldValue) nvml.Return {
					require.Len(t, values, 1)
					values[0].ValueType = uint32(nvml.VALUE_TYPE_UNSIGNED_INT)
					values[0].Value = [8]byte{}
					return nvml.SUCCESS
				}
				return device
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDevice := setupMockDeviceWithLibOpts(t, tt.customize)
			_, err := newNVLinkCollector(mockDevice, nil)
			require.ErrorIs(t, err, errUnsupportedDevice)
		})
	}
}

func makePLRResponseBytes(seed uint64) []byte {
	payload := make([]byte, ppcntSizeBytes)
	binary.BigEndian.PutUint32(payload[0:4], ppcntGroupPLR)
	offset := 8
	for i := range plrCounterFields {
		value := seed + uint64(i)
		binary.BigEndian.PutUint32(payload[offset:offset+4], uint32(value>>32))
		offset += 4
		binary.BigEndian.PutUint32(payload[offset:offset+4], uint32(value))
		offset += 4
	}
	return packTLV(ppcntRegID, ppcntSizeBytes, payload)
}

func TestPLRMetricSpecEntries(t *testing.T) {
	for _, metricName := range plrCounterFields {
		t.Run(metricName, func(t *testing.T) {
			spec, err := gpuspec.LoadMetricsSpec()
			require.NoError(t, err)

			metricSpec, ok := spec.Metrics[metricName]
			require.True(t, ok, "metric %s missing from spec", metricName)
			require.Contains(t, metricSpec.CustomTags, "nvlink_port")
			require.True(t, metricSpec.SupportsDeviceMode(gpuspec.DeviceModePhysical))
			require.False(t, metricSpec.SupportsDeviceMode(gpuspec.DeviceModeMIG))
			require.False(t, metricSpec.SupportsDeviceMode(gpuspec.DeviceModeVGPU))
		})
	}
}

func containsTag(tags []string, expected string) bool {
	for _, tag := range tags {
		if tag == expected {
			return true
		}
	}
	return false
}
