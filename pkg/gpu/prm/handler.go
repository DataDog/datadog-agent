// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package prm

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	sputils "github.com/DataDog/datadog-agent/pkg/system-probe/utils"
)

// Handler serves privileged PRM requests over the system-probe HTTP API.
type Handler struct {
	deviceCache ddnvml.DeviceCache
}

// NewHandler creates a PRM HTTP handler backed by the given device cache.
func NewHandler(deviceCache ddnvml.DeviceCache) *Handler {
	return &Handler{deviceCache: deviceCache}
}

// HandlePRMMetrics executes the requested PRM queries and returns one response per request.
func (h *Handler) HandlePRMMetrics(w http.ResponseWriter, req *http.Request) {
	var requests []model.PRMRequest
	if err := json.NewDecoder(req.Body).Decode(&requests); err != nil {
		http.Error(w, fmt.Sprintf("decode PRM requests: %v", err), http.StatusBadRequest)
		return
	}

	responses := make([]model.PRMResponse, 0, len(requests))
	for _, request := range requests {
		response := model.PRMResponse{Request: request}

		device, err := h.deviceCache.GetByUUID(request.DeviceUUID)
		if err != nil {
			response.Error = fmt.Sprintf("get device %s: %v", request.DeviceUUID, err)
			responses = append(responses, response)
			continue
		}

		arch, err := device.GetArchitecture()
		if err != nil {
			response.Error = fmt.Sprintf("get architecture for %s: %v", request.DeviceUUID, err)
			responses = append(responses, response)
			continue
		}

		if arch == nvml.DEVICE_ARCH_UNKNOWN || arch < nvml.DEVICE_ARCH_BLACKWELL {
			response.Error = fmt.Sprintf("device %s has unsupported architecture %v for PRM queries", request.DeviceUUID, arch)
			responses = append(responses, response)
			continue
		}

		counters, err := QueryPortCounters(device, request.Group, request.Port)
		if err != nil {
			response.Error = fmt.Sprintf("query PRM counters for %s port %d group 0x%x: %v", request.DeviceUUID, request.Port, request.Group, err)
			responses = append(responses, response)
			continue
		}

		response.Counters = counters
		responses = append(responses, response)
	}

	sputils.WriteAsJSON(req, w, responses, sputils.CompactOutput)
}
