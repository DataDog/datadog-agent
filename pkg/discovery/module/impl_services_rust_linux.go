// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Rust-backed getServices implementation using the libdd_discovery shared library.

//go:build dd_discovery_rust && cgo

package module

/*
#cgo CFLAGS:  -I${SRCDIR}/rust/include
#cgo LDFLAGS: -L${SRCDIR}/rust -L${SRCDIR}/rust/target/release -ldd_discovery
#include "dd_discovery.h"
*/
import "C"

import (
	"math"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/discovery/core"
	"github.com/DataDog/datadog-agent/pkg/discovery/model"
)

// getServicesRust invokes the Rust library to process categorized PID lists and returns
// service information. The caller must hold s.mux before calling this function.
func (s *discovery) getServicesRust(params core.Params) (*model.ServicesResponse, error) {
	return rustGetServices(params.NewPids, params.HeartbeatPids), nil
}

// rustGetServices invokes the Rust library with the given PID lists and copies
// the response into Go memory.
func rustGetServices(newPids, hbPids []int32) *model.ServicesResponse {
	var newPidsPtr *C.int32_t
	var newPidsLen C.size_t
	if len(newPids) > 0 {
		newPidsPtr = (*C.int32_t)(unsafe.Pointer(&newPids[0]))
		newPidsLen = C.size_t(len(newPids))
	}

	var hbPidsPtr *C.int32_t
	var hbPidsLen C.size_t
	if len(hbPids) > 0 {
		hbPidsPtr = (*C.int32_t)(unsafe.Pointer(&hbPids[0]))
		hbPidsLen = C.size_t(len(hbPids))
	}

	result := C.dd_discovery_get_services(newPidsPtr, newPidsLen, hbPidsPtr, hbPidsLen)
	if result == nil {
		return &model.ServicesResponse{Services: make([]model.Service, 0)}
	}
	defer C.dd_discovery_free(result)

	return convertNativeResult(result)
}

// convertNativeResult translates a C dd_discovery_result into a ServicesResponse.
func convertNativeResult(result *C.struct_dd_discovery_result) *model.ServicesResponse {
	response := &model.ServicesResponse{
		Services: make([]model.Service, 0, int(result.services_len)),
	}

	if cServices := sliceFromC(result.services, result.services_len); len(cServices) > 0 {
		for i := range cServices {
			response.Services = append(response.Services, convertRustService(&cServices[i]))
		}
	}

	if cPids := sliceFromC(result.injected_pids, result.injected_pids_len); len(cPids) > 0 {
		response.InjectedPIDs = make([]int, len(cPids))
		for i, pid := range cPids {
			response.InjectedPIDs[i] = int(pid)
		}
	}

	if cPids := sliceFromC(result.gpu_pids, result.gpu_pids_len); len(cPids) > 0 {
		response.GPUPIDs = make([]int, len(cPids))
		for i, pid := range cPids {
			response.GPUPIDs[i] = int(pid)
		}
	}

	return response
}

func sliceFromC[T any](data *T, length C.size_t) []T {
	if data == nil || length == 0 {
		return nil
	}
	return unsafe.Slice(data, length)
}

// fromDDStr converts a length-delimited C dd_str to a Go string.
func fromDDStr(s C.struct_dd_str) string {
	if s.data == nil || s.len == 0 {
		return ""
	}
	if s.len > math.MaxInt32 {
		return ""
	}
	return C.GoStringN(s.data, C.int(s.len))
}

// convertRustService converts a C dd_service to a model.Service, copying all
// fields into Go memory.
func convertRustService(svc *C.struct_dd_service) model.Service {
	result := model.Service{
		PID:                 int(svc.pid),
		GeneratedName:       fromDDStr(svc.generated_name),
		GeneratedNameSource: fromDDStr(svc.generated_name_source),
		APMInstrumentation:  bool(svc.apm_instrumentation),
		Language:            fromDDStr(svc.language),
		UST: model.UST{
			Service: fromDDStr(svc.ust.service),
			Env:     fromDDStr(svc.ust.env),
			Version: fromDDStr(svc.ust.version),
		},
	}

	if names := sliceFromC(svc.additional_generated_names.data, svc.additional_generated_names.len); len(names) > 0 {
		result.AdditionalGeneratedNames = make([]string, len(names))
		for i, n := range names {
			result.AdditionalGeneratedNames[i] = fromDDStr(n)
		}
	}

	if metas := sliceFromC(svc.tracer_metadata.data, svc.tracer_metadata.len); len(metas) > 0 {
		for _, m := range metas {
			result.TracerMetadata = append(result.TracerMetadata, buildTracerMetadata(
				uint8(m.schema_version),
				fromDDStr(m.runtime_id),
				fromDDStr(m.tracer_language),
				fromDDStr(m.tracer_version),
				fromDDStr(m.hostname),
				fromDDStr(m.service_name),
				fromDDStr(m.service_env),
				fromDDStr(m.service_version),
				fromDDStr(m.process_tags),
				fromDDStr(m.container_id),
				bool(m.logs_collected),
			))
		}
	}

	if ports := sliceFromC(svc.tcp_ports.data, svc.tcp_ports.len); len(ports) > 0 {
		result.TCPPorts = make([]uint16, len(ports))
		for i, p := range ports {
			result.TCPPorts[i] = uint16(p)
		}
	}

	if ports := sliceFromC(svc.udp_ports.data, svc.udp_ports.len); len(ports) > 0 {
		result.UDPPorts = make([]uint16, len(ports))
		for i, p := range ports {
			result.UDPPorts[i] = uint16(p)
		}
	}

	if logs := sliceFromC(svc.log_files.data, svc.log_files.len); len(logs) > 0 {
		result.LogFiles = make([]string, len(logs))
		for i, l := range logs {
			result.LogFiles[i] = fromDDStr(l)
		}
	}

	return result
}
