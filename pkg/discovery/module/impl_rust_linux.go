// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build cgo

package module

/*
#cgo CFLAGS:  -I${SRCDIR}/rust/include
#cgo LDFLAGS: -L${SRCDIR}/rust -L${SRCDIR}/rust/target/release -ldd_discovery
#include "dd_discovery.h"
*/
import "C"

import (
	"errors"
	"math"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/discovery/core"
	"github.com/DataDog/datadog-agent/pkg/discovery/model"
	tracermetadata "github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata/model"
)

// errRustLibraryPanicked is returned when libdd_discovery caught a Rust panic
// at the FFI boundary and returned NULL. Surfacing this as an error lets
// handleServices log the failure and return 500 rather than silently replying
// with an empty payload.
var errRustLibraryPanicked = errors.New("libdd_discovery returned NULL (panic caught at FFI boundary)")

// rustGetServices invokes the Rust library with the given PID lists and
// copies the response into Go-owned memory.
func rustGetServices(params core.Params) (*model.ServicesResponse, error) {
	newPidsPtr, newPidsLen := pidSlice(params.NewPids)
	hbPidsPtr, hbPidsLen := pidSlice(params.HeartbeatPids)

	result := C.dd_discovery_get_services(newPidsPtr, newPidsLen, hbPidsPtr, hbPidsLen)
	if result == nil {
		return nil, errRustLibraryPanicked
	}
	defer C.dd_discovery_free(result)

	response := &model.ServicesResponse{
		Services: make([]model.Service, 0, int(result.services_len)),
	}

	for _, svc := range sliceFromC(result.services, result.services_len) {
		response.Services = append(response.Services, convertRustService(&svc))
	}

	for _, pid := range sliceFromC(result.injected_pids, result.injected_pids_len) {
		response.InjectedPIDs = append(response.InjectedPIDs, int(pid))
	}

	for _, pid := range sliceFromC(result.gpu_pids, result.gpu_pids_len) {
		response.GPUPIDs = append(response.GPUPIDs, int(pid))
	}

	return response, nil
}

func pidSlice(pids []int32) (*C.int32_t, C.size_t) {
	if len(pids) == 0 {
		return nil, 0
	}
	return (*C.int32_t)(unsafe.Pointer(&pids[0])), C.size_t(len(pids))
}

func sliceFromC[T any](data *T, length C.size_t) []T {
	if data == nil || length == 0 {
		return nil
	}
	return unsafe.Slice(data, length)
}

func fromDDStr(s C.struct_dd_str) string {
	if s.data == nil || s.len == 0 {
		return ""
	}
	if s.len > math.MaxInt32 {
		return ""
	}
	return C.GoStringN(s.data, C.int(s.len))
}

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

	for _, m := range sliceFromC(svc.tracer_metadata.data, svc.tracer_metadata.len) {
		result.TracerMetadata = append(result.TracerMetadata, tracermetadata.TracerMetadata{
			SchemaVersion:  uint8(m.schema_version),
			RuntimeID:      fromDDStr(m.runtime_id),
			TracerLanguage: fromDDStr(m.tracer_language),
			TracerVersion:  fromDDStr(m.tracer_version),
			Hostname:       fromDDStr(m.hostname),
			ServiceName:    fromDDStr(m.service_name),
			ServiceEnv:     fromDDStr(m.service_env),
			ServiceVersion: fromDDStr(m.service_version),
			ProcessTags:    fromDDStr(m.process_tags),
			ContainerID:    fromDDStr(m.container_id),
			LogsCollected:  bool(m.logs_collected),
		})
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
