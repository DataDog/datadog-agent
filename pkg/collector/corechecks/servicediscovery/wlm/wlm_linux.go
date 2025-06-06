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

type fakeprocess struct {
	pid       int
	container *workloadmeta.Container
}

// getServices returns a list of processes that are running and have a
// container.  This is temporary until we have a way to get the services from
// the process collector.
func (d *DiscoveryWLM) getServices() ([]fakeprocess, error) {
	containers := d.wmeta.ListContainersWithFilter(func(container *workloadmeta.Container) bool {
		return container.State.Running && container.PID > 0
	})

	procs := make([]fakeprocess, 0, len(containers))
	for _, container := range containers {
		procs = append(procs, fakeprocess{
			pid:       int(container.PID),
			container: container,
		})
	}

	return procs, nil
}

// DiscoverServices discovers services from the workloadmeta component.
func (d *DiscoveryWLM) DiscoverServices() (*model.ServicesResponse, error) {
	procs, err := d.getServices()
	if err != nil {
		return nil, err
	}

	log.Info("procs discovered", "procs", procs)

	procMap := make(map[int32]*fakeprocess)
	pids := make([]int32, 0, len(procs))
	for _, proc := range procs {
		pids = append(pids, int32(proc.pid))
		procMap[int32(proc.pid)] = &proc
	}

	resp, err := d.discoveryCore.GetServices(core.Params{}, pids, nil, func(_ any, pid int32) *model.Service {
		info, ok := d.discoveryCore.Cache[pid]
		if ok {
			return info.ToModelService(pid, &model.Service{})
		}

		info = &core.ServiceInfo{
			Service: model.Service{
				ContainerID: procMap[pid].container.ID,
			},
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
