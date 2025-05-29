// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package module

import (
	"time"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DiscoveryWLM is the implementation of the service discovery check based on the workloadmeta component.
type DiscoveryWLM struct {
	wmeta         workloadmeta.Component
	discoveryCore *discoveryCore
}

// NewDiscoveryWLM creates a new DiscoveryWLM instance.
func NewDiscoveryWLM(store workloadmeta.Component, tagger tagger.Component) (*DiscoveryWLM, error) {
	config := newConfig()
	network, err := newNetworkCollector(config)
	if err != nil {
		network = nil
	}

	return &DiscoveryWLM{
		wmeta: store,
		discoveryCore: &discoveryCore{
			config:            config,
			cache:             make(map[int32]*serviceInfo),
			potentialServices: make(pidSet),
			runningServices:   make(pidSet),
			ignorePids:        make(pidSet),
			wmeta:             store,
			tagger:            tagger,
			timeProvider:      realTime{},
			network:           network,
			networkErrorLimit: log.NewLogLimit(10, 10*time.Minute),
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

	resp, err := d.discoveryCore.getServices(params{}, pids, nil, func(_ any, pid int32) *model.Service {
		info, ok := d.discoveryCore.cache[pid]
		if ok {
			return info.toModelService(pid, &model.Service{})
		}

		info = &serviceInfo{
			containerID: procMap[pid].container.ID,
		}
		d.discoveryCore.cache[pid] = info
		return info.toModelService(pid, &model.Service{})
	})
	if err != nil {
		return nil, err
	}

	log.Info("services discovered", "services", resp)

	return resp, nil
}

// Close closes the discovery module.
func (d *DiscoveryWLM) Close() {
	d.discoveryCore.close()
}
