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
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
)

const testGPUUUID = "GPU-00000000-1234-1234-1234-123456789012"

func TestPRMMetricsEndpoint(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		handler, uuid := setupHandler(nvml.DEVICE_ARCH_BLACKWELL, func(device *testDevice) {
			device.readWritePRM = func(buffer *nvml.PRMTLV_v1) error {
				port := int(binary.BigEndian.Uint32(buffer.InData[20:24]) >> 16)
				copy(buffer.InData[:], makePLRResponseBytes(uint64(port*100)))
				return nil
			}
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
		handler, _ := setupHandler(nvml.DEVICE_ARCH_BLACKWELL, nil)

		response := performRequest(t, handler, []model.PRMRequest{
			{DeviceUUID: "GPU-missing", Port: 1, Group: PPCNTGroupPLR},
		})

		require.Len(t, response, 1)
		require.Empty(t, response[0].Counters)
		require.Contains(t, response[0].Error, "get device GPU-missing")
	})

	t.Run("partial failure", func(t *testing.T) {
		handler, uuid := setupHandler(nvml.DEVICE_ARCH_BLACKWELL, func(device *testDevice) {
			device.readWritePRM = func(buffer *nvml.PRMTLV_v1) error {
				port := int(binary.BigEndian.Uint32(buffer.InData[20:24]) >> 16)
				if port == 2 {
					return nvml.ERROR_NOT_SUPPORTED
				}
				copy(buffer.InData[:], makePLRResponseBytes(123))
				return nil
			}
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
		handler, _ := setupHandler(nvml.DEVICE_ARCH_BLACKWELL, nil)
		response := performRequest(t, handler, []model.PRMRequest{})
		require.Empty(t, response)
	})

	t.Run("malformed json", func(t *testing.T) {
		handler, _ := setupHandler(nvml.DEVICE_ARCH_BLACKWELL, nil)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/gpu/prm-metrics", bytes.NewBufferString("{"))
		handler.HandlePRMMetrics(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("pre blackwell architecture", func(t *testing.T) {
		handler, uuid := setupHandler(nvml.DEVICE_ARCH_HOPPER, nil)

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

func setupHandler(arch nvml.DeviceArchitecture, customize func(device *testDevice)) (*Handler, string) {
	device := &testDevice{arch: arch}
	if customize != nil {
		customize(device)
	}

	return NewHandler(func(uuid string) (Device, error) {
		if uuid != testGPUUUID {
			return nil, errDeviceNotFound
		}
		return device, nil
	}), testGPUUUID
}
