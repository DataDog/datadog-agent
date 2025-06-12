// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package wlm provides workload metadata functionality for service discovery.
package wlm

import (
	"time"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/core"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DiscoveryWLM is the implementation of the service discovery check based on the workloadmeta component.
type DiscoveryWLM struct {
	wmeta         workloadmeta.Component
	discoveryCore *core.Discovery
}

// NewDiscoveryWLM creates a new DiscoveryWLM instance.
func NewDiscoveryWLM(store workloadmeta.Component, tagger tagger.Component) (*DiscoveryWLM, error) {
	config := core.NewConfig()
	return &DiscoveryWLM{
		wmeta: store,
		discoveryCore: &core.Discovery{
			Config:            config,
			Cache:             make(map[int32]*core.ServiceInfo),
			PotentialServices: make(core.PidSet),
			RunningServices:   make(core.PidSet),
			IgnorePids:        make(core.PidSet),
			WMeta:             store,
			Tagger:            tagger,
			TimeProvider:      core.RealTime{},
			Network:           nil,
			NetworkErrorLimit: log.NewLogLimit(10, 10*time.Minute),
		},
	}, nil
}

// convertWLMServiceToModelService converts workloadmeta.Service to model.Service
func convertWLMServiceToModelService(wlmService *workloadmeta.Service, pid int) *model.Service {
	if wlmService == nil {
		return nil
	}

	return &model.Service{
		PID:                        pid,
		GeneratedName:              wlmService.GeneratedName,
		GeneratedNameSource:        wlmService.GeneratedNameSource,
		AdditionalGeneratedNames:   wlmService.AdditionalGeneratedNames,
		ContainerServiceName:       wlmService.ContainerServiceName,
		ContainerServiceNameSource: wlmService.ContainerServiceNameSource,
		ContainerTags:              wlmService.ContainerTags,
		TracerMetadata:             wlmService.TracerMetadata,
		DDService:                  wlmService.DDService,
		DDServiceInjected:          wlmService.DDServiceInjected,
		CheckedContainerData:       wlmService.CheckedContainerData,
		Ports:                      wlmService.Ports,
		APMInstrumentation:         wlmService.APMInstrumentation,
		Language:                   wlmService.Language,
		Type:                       wlmService.Type,
		CommandLine:                wlmService.CommandLine,
		StartTimeMilli:             wlmService.StartTimeMilli,
		ContainerID:                wlmService.ContainerID,
		LastHeartbeat:              wlmService.LastHeartbeat,
	}
}

// getServices returns a list of processes that have service information set.
func (d *DiscoveryWLM) getServices() ([]*workloadmeta.Process, error) {
	processes := d.wmeta.ListProcessesWithFilter(func(process *workloadmeta.Process) bool {
		return process.Service != nil
	})

	return processes, nil
}

// DiscoverServices discovers services from the workloadmeta component.
func (d *DiscoveryWLM) DiscoverServices() (*model.ServicesResponse, error) {
	procs, err := d.getServices()
	if err != nil {
		return nil, err
	}

	log.Info("procs discovered", "procs", procs)

	procMap := make(map[int32]*workloadmeta.Process)
	pids := make([]int32, 0, len(procs))
	for _, proc := range procs {
		pids = append(pids, proc.Pid)
		procMap[proc.Pid] = proc
	}

	resp, err := d.discoveryCore.GetServices(core.Params{}, pids, nil, func(_ any, pid int32) *model.Service {
		info, ok := d.discoveryCore.Cache[pid]
		if ok {
			return info.ToModelService(pid, &model.Service{})
		}

		process := procMap[pid]
		if process == nil || process.Service == nil {
			return nil
		}

		// Convert the WLM service to a model.Service
		modelService := convertWLMServiceToModelService(process.Service, int(pid))
		if modelService == nil {
			return nil
		}

		info = &core.ServiceInfo{
			Service: *modelService,
		}
		d.discoveryCore.Cache[pid] = info
		return info.ToModelService(pid, &model.Service{})
	})
	if err != nil {
		return nil, err
	}

	log.Info("services discovered", "services", resp)

	return resp, nil
}

// Close closes the discovery module.
func (d *DiscoveryWLM) Close() {
	d.discoveryCore.Close()
}
