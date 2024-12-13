// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package servicediscovery

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	sysprobeclient "github.com/DataDog/datadog-agent/cmd/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

//go:generate mockgen -source=$GOFILE -package=$GOPACKAGE -destination=impl_linux_mock.go

func init() {
	newOSImpl = newLinuxImpl
}

type linuxImpl struct {
	getDiscoveryServices func(client *http.Client) (*model.ServicesResponse, error)
	time                 timer

	ignoreCfg map[string]bool

	ignoreProcs       map[int]bool
	aliveServices     map[int]*model.Service
	potentialServices map[int]*model.Service

	sysProbeClient *http.Client
}

func newLinuxImpl(ignoreCfg map[string]bool) (osImpl, error) {
	return &linuxImpl{
		getDiscoveryServices: getDiscoveryServices,
		time:                 realTime{},
		ignoreCfg:            ignoreCfg,
		ignoreProcs:          make(map[int]bool),
		aliveServices:        make(map[int]*model.Service),
		potentialServices:    make(map[int]*model.Service),
		sysProbeClient:       sysprobeclient.Get(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")),
	}, nil
}

func getDiscoveryServices(client *http.Client) (*model.ServicesResponse, error) {
	url := sysprobeclient.ModuleURL(sysconfig.DiscoveryModule, "/services")
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("got non-success status code: url: %s, status_code: %d", req.URL, resp.StatusCode)
	}

	res := &model.ServicesResponse{}
	if err := json.NewDecoder(resp.Body).Decode(res); err != nil {
		return nil, err
	}
	return res, nil
}

func (li *linuxImpl) DiscoverServices() (*discoveredServices, error) {
	response, err := li.getDiscoveryServices(li.sysProbeClient)
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

	li.handlePotentialServices(&events, serviceMap)

	// check open ports - these will be potential new services if they are still alive in the next iteration.
	for _, service := range response.Services {
		pid := service.PID
		if li.ignoreProcs[pid] {
			continue
		}
		if _, ok := li.aliveServices[pid]; !ok {
			log.Debugf("[pid: %d] found new process with open ports", pid)

			if li.ignoreCfg[service.Name] {
				log.Debugf("[pid: %d] process ignored from config: %s", pid, service.Name)
				li.ignoreProcs[pid] = true
				continue
			}

			log.Debugf("[pid: %d] adding process to potential: %s", pid, service.Name)
			li.potentialServices[pid] = &service
		}
	}

	// check if services previously marked as alive still are.
	for pid, svc := range li.aliveServices {
		if service, ok := serviceMap[pid]; !ok {
			delete(li.aliveServices, pid)
			events.stop = append(events.stop, *svc)
		} else if serviceHeartbeatTime := time.Unix(svc.LastHeartbeat, 0); now.Sub(serviceHeartbeatTime).Truncate(time.Minute) >= heartbeatTime {
			svc.LastHeartbeat = now.Unix()
			svc.RSS = service.RSS
			svc.CPUCores = service.CPUCores
			svc.ContainerID = service.ContainerID
			svc.GeneratedName = service.GeneratedName
			svc.ContainerServiceName = service.ContainerServiceName
			svc.ContainerServiceNameSource = service.ContainerServiceNameSource
			svc.Name = service.Name
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

// handlePotentialServices checks cached potential services we have seen in the
// previous call of the check. If they are still alive, start events are sent
// for these services.
func (li *linuxImpl) handlePotentialServices(events *serviceEvents, serviceMap map[int]*model.Service) {
	if len(li.potentialServices) == 0 {
		return
	}

	// potentialServices contains processes that we scanned in the previous
	// iteration and had open ports. We check if they are still alive in this
	// iteration, and if so, we send a start-service telemetry event.
	for pid, potential := range li.potentialServices {
		if service, ok := serviceMap[pid]; ok {
			potential.LastHeartbeat = service.LastHeartbeat
			potential.RSS = service.RSS
			potential.CPUCores = service.CPUCores
			potential.ContainerID = service.ContainerID
			potential.GeneratedName = service.GeneratedName
			potential.ContainerServiceName = service.ContainerServiceName
			potential.ContainerServiceNameSource = service.ContainerServiceNameSource
			potential.Name = service.Name

			li.aliveServices[pid] = potential
			events.start = append(events.start, *potential)
		}
	}
	clear(li.potentialServices)
}
