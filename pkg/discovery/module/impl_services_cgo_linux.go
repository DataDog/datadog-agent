// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// This file is compiled only when CGO is enabled (CGO_ENABLED=1, the default
// for Linux system-probe builds).  It wraps the Rust libdd_discovery shared
// library and provides the getServices method used by the discovery HTTP
// handler.
//
// Build requirement: the Rust shared library must be present at one of the
// following search paths before linking:
//
//	${SRCDIR}/rust/               (canonical — produced by build_rust_binaries)
//	${SRCDIR}/rust/target/release (cargo --release fallback for local dev)
//	${SRCDIR}/rust/target/debug   (cargo debug fallback for local dev)
//
// In the CI / system-probe build pipeline the library is produced by:
//
//	dda inv system-probe.build-object-files
//
// which calls build_rust_binaries() (Bazel) and installs libdd_discovery.so
// into pkg/discovery/module/rust/ via the :install Bazel target.
// That path is the same directory as ${SRCDIR}/rust/ resolved at link time.
//
// For local development without a full Bazel toolchain, the cargo fallback
// paths are available:
//
//	cd pkg/discovery/module/rust && cargo build --lib           # debug
//	cd pkg/discovery/module/rust && cargo build --lib --release # production

//go:build cgo

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

// getServices calls the Rust libdd_discovery shared library through CGO to
// perform service discovery.  It applies the same post-processing as the
// pure-Go implementation to maintain behavioural parity:
//
//  1. shouldIgnoreComm is pre-applied to HeartbeatPids and post-applied to
//     the services returned for NewPids so that ignored processes never appear
//     in Services.  InjectedPIDs is intentionally not filtered: the original
//     Go code runs detectAPMInjectorFromMaps before the shouldIgnoreComm
//     check, so injected-PID detection is independent of comm filtering.
//
//  2. service_type is computed by servicetype.Detect on the Go side only for
//     new-PID services.  The Rust library always returns an empty string for
//     this field.  Heartbeat services intentionally carry no Type, matching
//     the original getHeartbeatServiceInfo behaviour (minimal fields only).
//
func (s *discovery) getServices(params core.Params) (*model.ServicesResponse, error) {
	s.mux.Lock()
	defer s.mux.Unlock()

	// HeartbeatPids: pre-filter by shouldIgnoreComm before handing to Rust.
	// This mirrors getHeartbeatServiceInfo which calls shouldIgnoreComm first.
	hbPids := params.HeartbeatPids
	if s.config.IgnoreComms != nil && len(params.HeartbeatPids) > 0 {
		filtered := make([]int32, 0, len(params.HeartbeatPids))
		for _, pid := range params.HeartbeatPids {
			if !s.shouldIgnoreComm(pid) {
				filtered = append(filtered, pid)
			}
		}
		hbPids = filtered
	}

	// Build a heartbeat PID set used below to skip servicetype.Detect for
	// heartbeat-sourced services.  The Rust library returns both new-PID and
	// heartbeat-PID services in a single flat array, so we must tag them here.
	hbPidSet := make(map[int32]struct{}, len(hbPids))
	for _, pid := range hbPids {
		hbPidSet[pid] = struct{}{}
	}

	// NewPids: pass the full list to Rust so that APM injector detection runs
	// for every PID.  The original code calls detectAPMInjectorFromMaps before
	// the shouldIgnoreComm check, so comm-ignored PIDs can still appear in
	// InjectedPIDs.  Post-filtering services (below) handles the Services list.
	var newPidsPtr *C.int32_t
	var newPidsLen C.size_t
	if len(params.NewPids) > 0 {
		newPidsPtr = (*C.int32_t)(unsafe.Pointer(&params.NewPids[0]))
		newPidsLen = C.size_t(len(params.NewPids))
	}

	var hbPidsPtr *C.int32_t
	var hbPidsLen C.size_t
	if len(hbPids) > 0 {
		hbPidsPtr = (*C.int32_t)(unsafe.Pointer(&hbPids[0]))
		hbPidsLen = C.size_t(len(hbPids))
	}

	// Call into the library.  The returned struct is heap-allocated by Rust and
	// must be freed with dd_discovery_free exactly once.
	result := C.dd_discovery_get_services(newPidsPtr, newPidsLen, hbPidsPtr, hbPidsLen)
	if result == nil {
		return &model.ServicesResponse{Services: make([]model.Service, 0)}, nil
	}
	// Register the free before reading any fields so we can never accidentally
	// return early and leak.  All data copies happen before this defer fires.
	defer C.dd_discovery_free(result)

	response := &model.ServicesResponse{
		Services: make([]model.Service, 0, int(result.services_len)),
	}

	// Iterate over services one at a time — streaming approach, not a bulk copy.
	// Post-filter by shouldIgnoreComm: the Rust library has no equivalent of
	// the Go-side ignored_command_names configuration.
	if result.services_len > 0 && result.services != nil {
		cServices := unsafe.Slice(result.services, result.services_len)
		for i := range cServices {
			svc := cgoConvertService(&cServices[i])
			if s.shouldIgnoreComm(int32(svc.PID)) {
				continue
			}
			// Heartbeat services carry no Type (mirrors getHeartbeatServiceInfo
			// which intentionally returns minimal fields).  Only new-PID
			// services get the servicetype.Detect classification.
			if _, isHeartbeat := hbPidSet[int32(svc.PID)]; !isHeartbeat {
				svc.Type = string(servicetype.Detect(svc.TCPPorts, svc.UDPPorts))
			}
			response.Services = append(response.Services, svc)
		}
	}

	// Copy injected PIDs (not comm-filtered — see comment above getServices).
	if result.injected_pids_len > 0 && result.injected_pids != nil {
		cPids := unsafe.Slice(result.injected_pids, result.injected_pids_len)
		response.InjectedPIDs = make([]int, len(cPids))
		for i, pid := range cPids {
			response.InjectedPIDs[i] = int(pid)
		}
	}

	return response, nil
}

// cgoStr converts a length-delimited C dd_str to a Go string.
func cgoStr(s C.struct_dd_str) string {
	if s.data == nil || s.len == 0 {
		return ""
	}
	return C.GoStringN(s.data, C.int(s.len))
}

// cgoConvertService converts a single C dd_service to a model.Service value.
// Every field is copied into Go memory via cgoStr / direct cast before this
// function returns, so the values remain valid after dd_discovery_free.
//
// service_type (Type) is NOT set here; it is computed by the caller in
// getServices, where it is known whether the service comes from a new PID
// (Type is set via servicetype.Detect) or a heartbeat PID (Type left empty,
// matching the getHeartbeatServiceInfo minimal-fields contract).
func cgoConvertService(svc *C.struct_dd_service) model.Service {
	result := model.Service{
		PID:                 int(svc.pid),
		GeneratedName:       cgoStr(svc.generated_name),
		GeneratedNameSource: cgoStr(svc.generated_name_source),
		APMInstrumentation: bool(svc.apm_instrumentation),
		Language:           cgoStr(svc.language),
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
