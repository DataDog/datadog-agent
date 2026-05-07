// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package prm

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

func TestPRMMetricsEndpoint(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		handler, uuid := setupHandler(t, "blackwell", func(device *mock.Device) *mock.Device {
			device.ReadWritePRM_v1Func = func(buffer *nvml.PRMTLV_v1) nvml.Return {
				port := int(binary.BigEndian.Uint32(buffer.InData[20:24]) >> 16)
				copy(buffer.InData[:], makePLRResponseBytes(uint64(port*100)))
				return nvml.SUCCESS
			}
			return device
		})

		response := performRequest(t, handler, []model.PRMRequest{
			{DeviceUUID: uuid, Port: 1, Group: PPCNTGroupPLR},
			{DeviceUUID: uuid, Port: 2, Group: PPCNTGroupPLR},
		})

		require.Len(t, response, 2)
		require.Empty(t, response[0].Error)
		require.Empty(t, response[1].Error)
		require.Equal(t, uint64(100), response[0].Counters[PLRCounterFields[0]])
		require.Equal(t, uint64(200), response[1].Counters[PLRCounterFields[0]])
	})

	t.Run("unknown device", func(t *testing.T) {
		handler, _ := setupHandler(t, "blackwell", nil)

		response := performRequest(t, handler, []model.PRMRequest{
			{DeviceUUID: "GPU-missing", Port: 1, Group: PPCNTGroupPLR},
		})

		require.Len(t, response, 1)
		require.Empty(t, response[0].Counters)
		require.Contains(t, response[0].Error, "get device GPU-missing")
	})

	t.Run("partial failure", func(t *testing.T) {
		handler, uuid := setupHandler(t, "blackwell", func(device *mock.Device) *mock.Device {
			device.ReadWritePRM_v1Func = func(buffer *nvml.PRMTLV_v1) nvml.Return {
				port := int(binary.BigEndian.Uint32(buffer.InData[20:24]) >> 16)
				if port == 2 {
					return nvml.ERROR_NOT_SUPPORTED
				}
				copy(buffer.InData[:], makePLRResponseBytes(123))
				return nvml.SUCCESS
			}
			return device
		})

		response := performRequest(t, handler, []model.PRMRequest{
			{DeviceUUID: uuid, Port: 1, Group: PPCNTGroupPLR},
			{DeviceUUID: uuid, Port: 2, Group: PPCNTGroupPLR},
		})

		require.Len(t, response, 2)
		require.Empty(t, response[0].Error)
		require.NotEmpty(t, response[1].Error)
		require.Empty(t, response[1].Counters)
	})

	t.Run("empty request list", func(t *testing.T) {
		handler, _ := setupHandler(t, "blackwell", nil)
		response := performRequest(t, handler, []model.PRMRequest{})
		require.Empty(t, response)
	})

	t.Run("malformed json", func(t *testing.T) {
		handler, _ := setupHandler(t, "blackwell", nil)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/gpu/prm-metrics", bytes.NewBufferString("{"))
		handler.HandlePRMMetrics(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("pre blackwell architecture", func(t *testing.T) {
		handler, uuid := setupHandler(t, "hopper", nil)

		response := performRequest(t, handler, []model.PRMRequest{
			{DeviceUUID: uuid, Port: 1, Group: PPCNTGroupPLR},
		})

		require.Len(t, response, 1)
		require.Contains(t, response[0].Error, "unsupported architecture")
	})
}

func performRequest(t *testing.T, handler *Handler, requests []model.PRMRequest) []model.PRMResponse {
	t.Helper()

	body, err := json.Marshal(requests)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/gpu/prm-metrics", bytes.NewReader(body))
	handler.HandlePRMMetrics(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var response []model.PRMResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	return response
}

func setupHandler(t *testing.T, arch string, customize func(device *mock.Device) *mock.Device) (*Handler, string) {
	t.Helper()

	nvmlMock := testutil.GetBasicNvmlMockWithOptions(
		testutil.WithMIGDisabled(),
		testutil.WithDeviceCount(1),
	)
	device := testutil.GetDeviceMock(0, testutil.WithMockAllDeviceFunctions())
	if customize != nil {
		device = customize(device)
	}
	deviceArch, major, minor := testutil.ArchNameToNVML(arch)
	device.GetArchitectureFunc = func() (nvml.DeviceArchitecture, nvml.Return) {
		return deviceArch, nvml.SUCCESS
	}
	device.GetCudaComputeCapabilityFunc = func() (int, int, nvml.Return) {
		return major, minor, nvml.SUCCESS
	}

	nvmlMock.DeviceGetHandleByIndexFunc = func(index int) (nvml.Device, nvml.Return) {
		if index == 0 {
			return device, nvml.SUCCESS
		}
		return nil, nvml.ERROR_INVALID_ARGUMENT
	}

	ddnvml.WithMockNVML(t, nvmlMock)
	deviceCache := ddnvml.NewDeviceCache()
	require.NoError(t, deviceCache.Refresh())

	return NewHandler(deviceCache), testutil.GPUUUIDs[0]
}
