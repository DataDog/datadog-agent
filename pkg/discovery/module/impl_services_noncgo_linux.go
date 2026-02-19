// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// This file provides the pure-Go implementation of getServices used when CGO
// is disabled (CGO_ENABLED=0).  When CGO is enabled the equivalent in
// impl_services_cgo_linux.go is compiled instead.

//go:build !cgo

package module

import (
	"github.com/DataDog/datadog-agent/pkg/discovery/core"
	"github.com/DataDog/datadog-agent/pkg/discovery/model"
)

// getServices processes categorized PID lists and returns service information for each.
// This is used by the /services endpoint which accepts explicit PID lists.
func (s *discovery) getServices(params core.Params) (*model.ServicesResponse, error) {
	s.mux.Lock()
	defer s.mux.Unlock()
	response := &model.ServicesResponse{
		Services: make([]model.Service, 0),
	}

	context := newParsingContext()

	// Process new PIDs with full service info collection
	for _, pid := range params.NewPids {
		// Check for APM injector even if process is not detected as a service
		if detectAPMInjectorFromMaps(pid) {
			response.InjectedPIDs = append(response.InjectedPIDs, int(pid))
		}

		service := s.getServiceWithoutRetry(context, pid)
		if service == nil {
			continue
		}
		response.Services = append(response.Services, *service)
	}

	// Process heartbeat PIDs with minimal updates (only ports and log files)
	for _, pid := range params.HeartbeatPids {
		service := s.getHeartbeatServiceInfo(context, pid)
		if service == nil {
			continue
		}
		response.Services = append(response.Services, *service)
	}

	return response, nil
}
