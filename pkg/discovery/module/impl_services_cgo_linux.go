// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// CGO-backed getServices implementation using the Rust libdd_discovery shared library.
// Without the dd_discovery_cgo build tag, impl_linux.go is compiled instead.

//go:build dd_discovery_cgo

package module

/*
#cgo CFLAGS:  -I${SRCDIR}/rust/include
#cgo LDFLAGS: -L${SRCDIR}/rust -L${SRCDIR}/rust/target/release -L${SRCDIR}/rust/target/debug -ldd_discovery
#include "dd_discovery.h"
*/
import "C"

import (
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/discovery/core"
	"github.com/DataDog/datadog-agent/pkg/discovery/model"
	"github.com/DataDog/datadog-agent/pkg/discovery/servicetype"
	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
)

func (s *discovery) getServices(params core.Params) (*model.ServicesResponse, error) {
	s.mux.Lock()
	defer s.mux.Unlock()

	hbPids := s.filterHeartbeatPids(params.HeartbeatPids)

	result := nativeGetServices(params.NewPids, hbPids)
	if result == nil {
		return &model.ServicesResponse{Services: make([]model.Service, 0)}, nil
	}
	defer C.dd_discovery_free(result)

	return s.convertNativeResult(result, hbPids), nil
}

// filterHeartbeatPids returns the subset of pids not matched by shouldIgnoreComm.
func (s *discovery) filterHeartbeatPids(pids []int32) []int32 {
	if s.config.IgnoreComms == nil || len(pids) == 0 {
		return pids
	}
	filtered := make([]int32, 0, len(pids))
	for _, pid := range pids {
		if !s.shouldIgnoreComm(pid) {
			filtered = append(filtered, pid)
		}
	}
	return filtered
}

// nativeGetServices invokes the Rust library with the given PID lists.
func nativeGetServices(newPids, hbPids []int32) *C.struct_dd_discovery_result {
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

	return C.dd_discovery_get_services(newPidsPtr, newPidsLen, hbPidsPtr, hbPidsLen)
}

// convertNativeResult translates a C dd_discovery_result into a ServicesResponse.
// For new PIDs it applies comm filtering and service type detection.
func (s *discovery) convertNativeResult(result *C.struct_dd_discovery_result, hbPids []int32) *model.ServicesResponse {
	hbPidSet := make(map[int32]struct{}, len(hbPids))
	for _, pid := range hbPids {
		hbPidSet[pid] = struct{}{}
	}

	response := &model.ServicesResponse{
		Services: make([]model.Service, 0, int(result.services_len)),
	}

	if result.services_len > 0 && result.services != nil {
		cServices := unsafe.Slice(result.services, result.services_len)
		for i := range cServices {
			svc := cgoConvertService(&cServices[i])
			_, isHeartbeat := hbPidSet[int32(svc.PID)]
			if !isHeartbeat && s.shouldIgnoreComm(int32(svc.PID)) {
				continue
			}
			if !isHeartbeat {
				svc.Type = string(servicetype.Detect(svc.TCPPorts, svc.UDPPorts))
			}
			response.Services = append(response.Services, svc)
		}
	}

	if result.injected_pids_len > 0 && result.injected_pids != nil {
		cPids := unsafe.Slice(result.injected_pids, result.injected_pids_len)
		response.InjectedPIDs = make([]int, len(cPids))
		for i, pid := range cPids {
			response.InjectedPIDs[i] = int(pid)
		}
	}

	return response
}

// cgoStr converts a length-delimited C dd_str to a Go string.
func cgoStr(s C.struct_dd_str) string {
	if s.data == nil || s.len == 0 {
		return ""
	}
	return C.GoStringN(s.data, C.int(s.len))
}

// cgoConvertService converts a C dd_service to a model.Service, copying all
// fields into Go memory. Type is not set; the caller applies servicetype.Detect
// for new-PID services and leaves it empty for heartbeat services.
func cgoConvertService(svc *C.struct_dd_service) model.Service {
	result := model.Service{
		PID:                 int(svc.pid),
		GeneratedName:       cgoStr(svc.generated_name),
		GeneratedNameSource: cgoStr(svc.generated_name_source),
		APMInstrumentation:  bool(svc.apm_instrumentation),
		Language:            cgoStr(svc.language),
		UST: model.UST{
			Service: cgoStr(svc.ust.service),
			Env:     cgoStr(svc.ust.env),
			Version: cgoStr(svc.ust.version),
		},
	}

	if svc.additional_generated_names.len > 0 && svc.additional_generated_names.data != nil {
		names := unsafe.Slice(svc.additional_generated_names.data, svc.additional_generated_names.len)
		result.AdditionalGeneratedNames = make([]string, len(names))
		for i, n := range names {
			result.AdditionalGeneratedNames[i] = cgoStr(n)
		}
	}

	if svc.tracer_metadata.len > 0 && svc.tracer_metadata.data != nil {
		metas := unsafe.Slice(svc.tracer_metadata.data, svc.tracer_metadata.len)
		result.TracerMetadata = make([]tracermetadata.TracerMetadata, len(metas))
		for i, m := range metas {
			result.TracerMetadata[i] = tracermetadata.TracerMetadata{
				SchemaVersion:  uint8(m.schema_version),
				RuntimeID:      cgoStr(m.runtime_id),
				TracerLanguage: cgoStr(m.tracer_language),
				TracerVersion:  cgoStr(m.tracer_version),
				Hostname:       cgoStr(m.hostname),
				ServiceName:    cgoStr(m.service_name),
				ServiceEnv:     cgoStr(m.service_env),
				ServiceVersion: cgoStr(m.service_version),
			}
		}
	}

	if svc.tcp_ports.len > 0 && svc.tcp_ports.data != nil {
		ports := unsafe.Slice(svc.tcp_ports.data, svc.tcp_ports.len)
		result.TCPPorts = make([]uint16, len(ports))
		for i, p := range ports {
			result.TCPPorts[i] = uint16(p)
		}
	}

	if svc.udp_ports.len > 0 && svc.udp_ports.data != nil {
		ports := unsafe.Slice(svc.udp_ports.data, svc.udp_ports.len)
		result.UDPPorts = make([]uint16, len(ports))
		for i, p := range ports {
			result.UDPPorts[i] = uint16(p)
		}
	}

	if svc.log_files.len > 0 && svc.log_files.data != nil {
		logs := unsafe.Slice(svc.log_files.data, svc.log_files.len)
		result.LogFiles = make([]string, len(logs))
		for i, l := range logs {
			result.LogFiles[i] = cgoStr(l)
		}
	}

	return result
}
