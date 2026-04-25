// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"

	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/metrics"
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
			switch len(values) {
			case 1:
				values[0].ValueType = uint32(nvml.VALUE_TYPE_UNSIGNED_INT)
				values[0].Value = [8]byte{2, 0, 0, 0, 0, 0, 0, 0}
			case len(nvlinkFECHistoryFieldIDs):
				scopeID := values[0].ScopeId
				for i := range values {
					require.Equal(t, scopeID, values[i].ScopeId)
					require.Equal(t, nvlinkFECHistoryFieldIDs[i], values[i].FieldId)
					values[i].NvmlReturn = uint32(nvml.SUCCESS)
					values[i].ValueType = uint32(nvml.VALUE_TYPE_UNSIGNED_INT)
					binary.LittleEndian.PutUint32(values[i].Value[:], uint32(int(scopeID)*100+10+i))
				}
			default:
				t.Fatalf("unexpected GetFieldValues request size: %d", len(values))
			}
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
	collectedMetrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, collectedMetrics, (len(plrCounterFields)+len(nvlinkFECHistoryFieldIDs))*2)

	port1Count := 0
	port2Count := 0
	fecMetricCount := 0
	for _, metric := range collectedMetrics {
		if metric.Name == nvlinkFECHistoryMetricName {
			require.Equal(t, metrics.HistogramType, metric.Type)
			require.NotNil(t, metric.HistogramBucket)
			require.True(t, metric.HistogramBucket.Monotonic)
			fecMetricCount++
		}
		switch {
		case hasTag(metric.Tags, "nvlink_port:1"):
			port1Count++
		case hasTag(metric.Tags, "nvlink_port:2"):
			port2Count++
		default:
			t.Fatalf("missing nvlink_port tag on metric %+v", metric)
		}
	}
	require.Equal(t, len(plrCounterFields)+len(nvlinkFECHistoryFieldIDs), port1Count)
	require.Equal(t, len(plrCounterFields)+len(nvlinkFECHistoryFieldIDs), port2Count)
	require.Equal(t, len(nvlinkFECHistoryFieldIDs)*2, fecMetricCount)
}

func TestNvlinkCollectorOnePortFails(t *testing.T) {
	callCountByPort := map[int]int{}
	stableCollector := func(port int) ([]Metric, error) {
		return []Metric{{
			Name:  "stable.metric",
			Value: float64(port),
			Type:  metrics.GaugeType,
		}}, nil
	}
	flakyCollector := func(port int) ([]Metric, error) {
		callCountByPort[port]++
		if port == 2 && callCountByPort[port] == 1 {
			return nil, errors.New("transient failure")
		}
		return []Metric{{
			Name:  "flaky.metric",
			Value: float64(port * 10),
			Type:  metrics.GaugeType,
		}}, nil
	}

	collector := &nvlinkCollector{
		portCollectors: map[int][]portCollector{
			1: {stableCollector, flakyCollector},
			2: {stableCollector, flakyCollector},
		},
	}
	require.Len(t, collector.portCollectors, 2)
	require.Len(t, collector.portCollectors[1], 2)
	require.Len(t, collector.portCollectors[2], 2)

	collectedMetrics, err := collector.Collect()
	require.Error(t, err)
	require.Len(t, collectedMetrics, 3)
	require.Equal(t, 1, callCountByPort[1])
	require.Equal(t, 1, callCountByPort[2])
	require.Len(t, collector.portCollectors, 2)
	require.Len(t, collector.portCollectors[1], 2)
	require.Len(t, collector.portCollectors[2], 2)

	metricsByPortAndName := map[string]map[string]float64{}
	for _, metric := range collectedMetrics {
		var portTag string
		for _, tag := range metric.Tags {
			if tag == "nvlink_port:1" || tag == "nvlink_port:2" {
				portTag = tag
				break
			}
		}
		require.NotEmpty(t, portTag, "missing nvlink port tag on metric %+v", metric)
		if _, ok := metricsByPortAndName[portTag]; !ok {
			metricsByPortAndName[portTag] = map[string]float64{}
		}
		metricsByPortAndName[portTag][metric.Name] = metric.Value
	}
	require.Equal(t, map[string]float64{"stable.metric": 1, "flaky.metric": 10}, metricsByPortAndName["nvlink_port:1"])
	require.Equal(t, map[string]float64{"stable.metric": 2}, metricsByPortAndName["nvlink_port:2"])

	collectedMetrics, err = collector.Collect()
	require.NoError(t, err)
	require.Len(t, collectedMetrics, 4)
	require.Equal(t, 2, callCountByPort[1])
	require.Equal(t, 2, callCountByPort[2])
	require.Len(t, collector.portCollectors, 2)
	require.Len(t, collector.portCollectors[1], 2)
	require.Len(t, collector.portCollectors[2], 2)

	metricsByPortAndName = map[string]map[string]float64{}
	for _, metric := range collectedMetrics {
		var portTag string
		for _, tag := range metric.Tags {
			if tag == "nvlink_port:1" || tag == "nvlink_port:2" {
				portTag = tag
				break
			}
		}
		require.NotEmpty(t, portTag, "missing nvlink port tag on metric %+v", metric)
		if _, ok := metricsByPortAndName[portTag]; !ok {
			metricsByPortAndName[portTag] = map[string]float64{}
		}
		metricsByPortAndName[portTag][metric.Name] = metric.Value
	}
	require.Equal(t, map[string]float64{"stable.metric": 1, "flaky.metric": 10}, metricsByPortAndName["nvlink_port:1"])
	require.Equal(t, map[string]float64{"stable.metric": 2, "flaky.metric": 20}, metricsByPortAndName["nvlink_port:2"])
}

func TestNVLinkCollectorFECScopesAndBuckets(t *testing.T) {
	type fieldRequest struct {
		fieldID uint32
		scopeID uint32
	}

	var requests []fieldRequest
	mockDevice := setupMockDeviceWithLibOpts(t, func(device *mock.Device) *mock.Device {
		testutil.WithMockAllDeviceFunctions()(device)
		device.GetFieldValuesFunc = func(values []nvml.FieldValue) nvml.Return {
			switch len(values) {
			case len(nvlinkFECHistoryFieldIDs):
				for i := range values {
					requests = append(requests, fieldRequest{fieldID: values[i].FieldId, scopeID: values[i].ScopeId})
					values[i].NvmlReturn = uint32(nvml.SUCCESS)
					values[i].ValueType = uint32(nvml.VALUE_TYPE_UNSIGNED_INT)
					binary.LittleEndian.PutUint32(values[i].Value[:], uint32(100+int(values[i].ScopeId)*10+i))
				}
			default:
				t.Fatalf("unexpected GetFieldValues request size: %d", len(values))
			}
			return nvml.SUCCESS
		}
		return device
	})

	collector := &nvlinkCollector{device: mockDevice}
	port1Metrics, err := collector.readFECHistory(1)
	require.NoError(t, err)
	port2Metrics, err := collector.readFECHistory(2)
	require.NoError(t, err)

	fecMetricsByPort := map[string][]Metric{}
	for _, metric := range port1Metrics {
		require.Equal(t, nvlinkFECHistoryMetricName, metric.Name)
		require.NotNil(t, metric.HistogramBucket)
		fecMetricsByPort["nvlink_port:1"] = append(fecMetricsByPort["nvlink_port:1"], metric)
	}
	for _, metric := range port2Metrics {
		require.Equal(t, nvlinkFECHistoryMetricName, metric.Name)
		require.NotNil(t, metric.HistogramBucket)
		fecMetricsByPort["nvlink_port:2"] = append(fecMetricsByPort["nvlink_port:2"], metric)
	}

	require.Len(t, fecMetricsByPort["nvlink_port:1"], len(nvlinkFECHistoryFieldIDs))
	require.Len(t, fecMetricsByPort["nvlink_port:2"], len(nvlinkFECHistoryFieldIDs))
	require.Len(t, requests, len(nvlinkFECHistoryFieldIDs)*2)

	requestsByScope := map[uint32][]uint32{}
	for _, request := range requests {
		requestsByScope[request.scopeID] = append(requestsByScope[request.scopeID], request.fieldID)
	}

	require.Equal(t, nvlinkFECHistoryFieldIDs, requestsByScope[0])
	require.Equal(t, nvlinkFECHistoryFieldIDs, requestsByScope[1])

	for idx, metric := range fecMetricsByPort["nvlink_port:1"] {
		require.Equal(t, float64(100+idx), metric.Value)
		require.Equal(t, [2]float64{float64(idx), float64(idx + 1)}, metric.HistogramBucket.Bounds)
		require.True(t, metric.HistogramBucket.Monotonic)
		require.False(t, metric.HistogramBucket.FlushFirstValue)
	}
	for idx, metric := range fecMetricsByPort["nvlink_port:2"] {
		require.Equal(t, float64(110+idx), metric.Value)
		require.Equal(t, [2]float64{float64(idx), float64(idx + 1)}, metric.HistogramBucket.Bounds)
		require.True(t, metric.HistogramBucket.Monotonic)
		require.False(t, metric.HistogramBucket.FlushFirstValue)
	}
}

func TestNVLinkCollectorUnsupportedDevice(t *testing.T) {
	tests := []struct {
		name      string
		customize func(*mock.Device) *mock.Device
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
	spec, err := gpuspec.LoadMetricsSpec()
	require.NoError(t, err)

	for _, metricName := range plrCounterFields {
		t.Run(metricName, func(t *testing.T) {
			metricSpec, ok := spec.Metrics[metricName]
			require.True(t, ok, "metric %s missing from spec", metricName)
			require.Contains(t, metricSpec.CustomTags, "nvlink_port")
			require.True(t, metricSpec.SupportsDeviceMode(gpuspec.DeviceModePhysical))
			require.False(t, metricSpec.SupportsDeviceMode(gpuspec.DeviceModeMIG))
			require.False(t, metricSpec.SupportsDeviceMode(gpuspec.DeviceModeVGPU))
		})
	}
}

func hasTag(tags []string, expected string) bool {
	for _, tag := range tags {
		if tag == expected {
			return true
		}
	}
	return false
}
