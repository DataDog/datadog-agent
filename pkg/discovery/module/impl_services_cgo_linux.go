// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// CGO-backed implementation using the Rust libdd_discovery shared library.
// When CGO is disabled, impl_linux.go is compiled instead.

//go:build cgo

package module

/*
#cgo CFLAGS:  -I${SRCDIR}/rust/include
#cgo LDFLAGS: -L${SRCDIR}/rust -L${SRCDIR}/rust/target/release -L${SRCDIR}/rust/target/debug -ldd_discovery
#include "dd_discovery.h"
*/
import "C"

import (
	"net/http"
	"sync"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/discovery/core"
	"github.com/DataDog/datadog-agent/pkg/discovery/model"
	"github.com/DataDog/datadog-agent/pkg/discovery/servicetype"
	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/privileged"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	pathServices = "/services"
)

// Ensure discovery implements the module.Module interface.
var _ module.Module = &discovery{}

// discovery is an implementation of the Module interface for the discovery module.
type discovery struct {
	core core.Discovery

	config *core.DiscoveryConfig

	mux *sync.RWMutex

	// privilegedDetector is used to detect the language of a process.
	privilegedDetector privileged.LanguageDetector
}

// NewDiscoveryModule creates a new discovery system probe module.
func NewDiscoveryModule(_ *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
	cfg := core.NewConfig()

	d := &discovery{
		core: core.Discovery{
			Config: cfg,
		},
		config:             cfg,
		mux:                &sync.RWMutex{},
		privilegedDetector: privileged.NewLanguageDetector(),
	}

	return d, nil
}

// GetStats returns the stats of the discovery module.
func (s *discovery) GetStats() map[string]any {
	return nil
}

// Register registers the discovery module with the provided HTTP mux.
func (s *discovery) Register(httpMux *module.Router) error {
	httpMux.HandleFunc("/status", s.handleStatusEndpoint)
	httpMux.HandleFunc("/state", s.handleStateEndpoint)
	httpMux.HandleFunc(pathServices, utils.WithConcurrencyLimit(utils.DefaultMaxConcurrentRequests, s.handleServices))

	return nil
}

// Close cleans resources used by the discovery module.
func (s *discovery) Close() {
	s.mux.Lock()
	defer s.mux.Unlock()

	s.core.Close()
}

// handleStatusEndpoint is the handler for the /status endpoint.
// Reports the status of the discovery module.
func (s *discovery) handleStatusEndpoint(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte("Discovery Module is running"))
}

// handleStateEndpoint is the handler for the /state endpoint.
// Returns the internal state of the discovery module.
func (s *discovery) handleStateEndpoint(w http.ResponseWriter, _ *http.Request) {
	s.mux.Lock()
	defer s.mux.Unlock()

	state := make(map[string]interface{})

	utils.WriteAsJSON(w, state, utils.CompactOutput)
}

func (s *discovery) handleServices(w http.ResponseWriter, req *http.Request) {
	params, err := core.ParseParamsFromRequest(req)
	if err != nil {
		_ = log.Errorf("invalid params to /discovery%s: %v", pathServices, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	services, err := s.getServices(params)
	if err != nil {
		_ = log.Errorf("failed to handle /discovery%s: %v", pathServices, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	utils.WriteAsJSON(w, services, utils.CompactOutput)
}

func (s *discovery) getServices(params core.Params) (*model.ServicesResponse, error) {
	s.mux.Lock()
	defer s.mux.Unlock()

	// Pre-filter heartbeat PIDs by comm name before passing to Rust.
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

	// Tag heartbeat PIDs so the loop below can skip servicetype.Detect for them.
	// Rust returns new-PID and heartbeat-PID services in a single flat array.
	hbPidSet := make(map[int32]struct{}, len(hbPids))
	for _, pid := range hbPids {
		hbPidSet[pid] = struct{}{}
	}

	// NewPids are not pre-filtered: comm-ignored PIDs can still appear in
	// InjectedPIDs, so the full list is passed to Rust and filtered afterwards.
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

	result := C.dd_discovery_get_services(newPidsPtr, newPidsLen, hbPidsPtr, hbPidsLen)
	if result == nil {
		return &model.ServicesResponse{Services: make([]model.Service, 0)}, nil
	}
	defer C.dd_discovery_free(result)

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

	return response, nil
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
