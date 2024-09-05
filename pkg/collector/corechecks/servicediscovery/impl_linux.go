// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package servicediscovery

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/servicetype"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	processnet "github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

//go:generate mockgen -source=$GOFILE -package=$GOPACKAGE -destination=impl_linux_mock.go

func init() {
	newOSImpl = newLinuxImpl
}

type linuxImpl struct {
	getSysProbeClient func() (systemProbeClient, error)
	time              timer

	ignoreCfg map[string]bool

	ignoreProcs       map[int]bool
	aliveServices     map[int]*serviceInfo
	potentialServices map[int]*serviceInfo
}

func newLinuxImpl(ignoreCfg map[string]bool) (osImpl, error) {
	return &linuxImpl{
		getSysProbeClient: getSysProbeClient,
		time:              realTime{},
		ignoreCfg:         ignoreCfg,
		ignoreProcs:       make(map[int]bool),
		aliveServices:     make(map[int]*serviceInfo),
		potentialServices: make(map[int]*serviceInfo),
	}, nil
}

func (li *linuxImpl) DiscoverServices() (*discoveredServices, error) {
	sysProbe, err := li.getSysProbeClient()
	if err != nil {
		return nil, errWithCode{
			err:  err,
			code: errorCodeSystemProbeConn,
		}
	}

	response, err := sysProbe.GetDiscoveryServices()
	if err != nil {
		return nil, errWithCode{
			err:  err,
			code: errorCodeSystemProbeServices,
		}
	}

	// The endpoint could be refactored in the future to return a map to avoid this.
	serviceMap := make(map[int]*model.Service, len(response.Services))
	for _, service := range response.Services {
		serviceMap[service.PID] = &service
	}

	events := serviceEvents{}

	now := li.time.Now()

	// potentialServices contains processes that we scanned in the previous iteration and had open ports.
	// we check if they are still alive in this iteration, and if so, we send a start-service telemetry event.
	for pid, svc := range li.potentialServices {
		if service, ok := serviceMap[pid]; ok {
			svc.LastHeartbeat = now
			svc.service.RSS = service.RSS
			svc.service.CPUCores = service.CPUCores
			li.aliveServices[pid] = svc
			events.start = append(events.start, *svc)
		}
	}
	clear(li.potentialServices)

	// check open ports - these will be potential new services if they are still alive in the next iteration.
	for _, service := range response.Services {
		pid := service.PID
		if li.ignoreProcs[pid] {
			continue
		}
		if _, ok := li.aliveServices[pid]; !ok {
			log.Debugf("[pid: %d] found new process with open ports", pid)

			svc := li.getServiceInfo(service)
			if li.ignoreCfg[svc.meta.Name] {
				log.Debugf("[pid: %d] process ignored from config: %s", pid, svc.meta.Name)
				li.ignoreProcs[pid] = true
				continue
			}
			log.Debugf("[pid: %d] adding process to potential: %s", pid, svc.meta.Name)
			li.potentialServices[pid] = &svc
		}
	}

	// check if services previously marked as alive still are.
	for pid, svc := range li.aliveServices {
		if service, ok := serviceMap[pid]; !ok {
			delete(li.aliveServices, pid)
			events.stop = append(events.stop, *svc)
		} else if now.Sub(svc.LastHeartbeat).Truncate(time.Minute) >= heartbeatTime {
			svc.LastHeartbeat = now
			svc.service.RSS = service.RSS
			svc.service.CPUCores = service.CPUCores
			events.heartbeat = append(events.heartbeat, *svc)
		}
	}

	// check if services previously marked as ignore are still alive.
	for pid := range li.ignoreProcs {
		if _, ok := serviceMap[pid]; !ok {
			delete(li.ignoreProcs, pid)
		}
	}

	return &discoveredServices{
		ignoreProcs:     li.ignoreProcs,
		potentials:      li.potentialServices,
		runningServices: li.aliveServices,
		events:          events,
	}, nil
}

func (li *linuxImpl) getServiceInfo(service model.Service) serviceInfo {
	// if the process name is docker-proxy, we should talk to docker to get the process command line and env vars
	// have to see how far this can go but not for the initial release

	// for now, docker-proxy is going on the ignore list

	serviceType := servicetype.Detect(service.Name, service.Ports)

	meta := ServiceMetadata{
		Name:               service.Name,
		Language:           service.Language,
		Type:               string(serviceType),
		APMInstrumentation: service.APMInstrumentation,
	}

	return serviceInfo{
		meta:          meta,
		service:       service,
		LastHeartbeat: li.time.Now(),
	}
}

type systemProbeClient interface {
	GetDiscoveryServices() (*model.ServicesResponse, error)
}

func getSysProbeClient() (systemProbeClient, error) {
	return processnet.GetRemoteSystemProbeUtil(
		ddconfig.SystemProbe().GetString("system_probe_config.sysprobe_socket"),
	)
}
