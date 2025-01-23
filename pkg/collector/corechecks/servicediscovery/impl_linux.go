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
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/servicetype"
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

	aliveServices     map[int]*serviceInfo
	potentialServices map[int]*serviceInfo

	sysProbeClient *http.Client
}

func newLinuxImpl() (osImpl, error) {
	return &linuxImpl{
		getDiscoveryServices: getDiscoveryServices,
		time:                 realTime{},
		aliveServices:        make(map[int]*serviceInfo),
		potentialServices:    make(map[int]*serviceInfo),
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

	li.handlePotentialServices(&events, now, serviceMap)

	// check open ports - these will be potential new services if they are still alive in the next iteration.
	for _, service := range response.Services {
		pid := service.PID
		if _, ok := li.aliveServices[pid]; !ok {
			log.Debugf("[pid: %d] found new process with open ports", pid)

			svc := li.getServiceInfo(service)
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
			svc.service.ContainerID = service.ContainerID
			svc.service.GeneratedName = service.GeneratedName
			svc.service.ContainerServiceName = service.ContainerServiceName
			svc.service.ContainerServiceNameSource = service.ContainerServiceNameSource
			svc.service.Name = service.Name
			svc.meta.Name = service.Name
			events.heartbeat = append(events.heartbeat, *svc)
		}
	}

	return &discoveredServices{
		potentials:      li.potentialServices,
		runningServices: li.aliveServices,
		events:          events,
	}, nil
}

// handlePotentialServices checks cached potential services we have seen in the
// previous call of the check. If they are still alive, start events are sent
// for these services.
func (li *linuxImpl) handlePotentialServices(events *serviceEvents, now time.Time, serviceMap map[int]*model.Service) {
	if len(li.potentialServices) == 0 {
		return
	}

	// potentialServices contains processes that we scanned in the previous
	// iteration and had open ports. We check if they are still alive in this
	// iteration, and if so, we send a start-service telemetry event.
	for pid, svc := range li.potentialServices {
		if service, ok := serviceMap[pid]; ok {
			svc.LastHeartbeat = now
			svc.service.RSS = service.RSS
			svc.service.CPUCores = service.CPUCores
			svc.service.ContainerID = service.ContainerID
			svc.service.GeneratedName = service.GeneratedName
			svc.service.ContainerServiceName = service.ContainerServiceName
			svc.service.ContainerServiceNameSource = service.ContainerServiceNameSource
			svc.service.Name = service.Name
			svc.meta.Name = service.Name

			li.aliveServices[pid] = svc
			events.start = append(events.start, *svc)
		}
	}
	clear(li.potentialServices)
}

func (li *linuxImpl) getServiceInfo(service model.Service) serviceInfo {
	// if the process name is docker-proxy, we should talk to docker to get the process command line and env vars
	// have to see how far this can go but not for the initial release

	// for now, docker-proxy is going on the ignore list

	serviceType := servicetype.Detect(service.Ports)

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
