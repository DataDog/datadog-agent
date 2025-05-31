// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/gopsutil/process"
	"go.uber.org/fx"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	collectorID  = "service"
	pullInterval = 30 * time.Second

	// The maximum number of times that we check if a process has open ports
	// before ignoring it forever.
	maxPortCheckTries = 10
)

type pidSet map[int32]struct{}

func (s pidSet) has(pid int32) bool {
	_, present := s[pid]
	return present
}

func (s pidSet) add(pid int32) {
	s[pid] = struct{}{}
}

// cleanPidSets deletes dead PIDs from the provided maps. This function is not
// thread-safe and it is up to the caller to ensure s.mux is locked.
func cleanPidMap[T any](alivePids pidSet, maps ...map[int32]T) {
	for _, m := range maps {
		for pid := range m {
			if alivePids.has(pid) {
				continue
			}

			delete(m, pid)
		}
	}
}

type collector struct {
	id      string
	catalog workloadmeta.AgentType
	store   workloadmeta.Component

	serviceRetries map[int32]uint
	ignoredPids    pidSet

	sysProbeClient *http.Client
	startTime      time.Time
	startupTimeout time.Duration
}

func NewCollector() (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &collector{
			id:             collectorID,
			catalog:        workloadmeta.NodeAgent,
			serviceRetries: make(map[int32]uint),
			ignoredPids:    make(pidSet),
			sysProbeClient: sysprobeclient.Get(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")),
			startTime:      time.Now(),
			startupTimeout: pkgconfigsetup.Datadog().GetDuration("check_system_probe_startup_time"),
		},
	}, nil
}

func GetFxOptions() fx.Option {
	return fx.Provide(NewCollector)
}

func (c *collector) Start(ctx context.Context, store workloadmeta.Component) error {
	log.Debugf("initializing Service Discovery collector")
	go func() {
		ticker := time.NewTicker(pullInterval)
		for {
			select {
			case <-ticker.C:
				c.updateServices()
			case <-ctx.Done():
				ticker.Stop()
				return
			}
		}
	}()

	return nil
}

func (c *collector) Pull(_ context.Context) error {
	return nil
}

func (c *collector) GetID() string {
	return c.id
}

func (c *collector) GetTargetCatalog() workloadmeta.AgentType {
	return c.catalog
}

func (c *collector) updateServices() {
	alivePids, err := getAlivePids()
	if err != nil {
		log.Errorf("failed to get alive pids: %s", err)
		return
	}

	pids, pidsToService := c.getPidsToRequest(alivePids)
	resp, err := c.getDiscoveryServices(pids)
	if err != nil {
		if time.Since(c.startTime) < c.startupTimeout {
			log.Warnf("service collector: system-probe not started yet: %v", err)
			return
		}

		log.Errorf("failed to get services: %s", err)
		return
	}

	for i, service := range resp.Services {
		pidsToService[int32(service.PID)] = &resp.Services[i]
		log.Debugf("found service: %+v", service)
	}

	for _, pid := range pids {
		if service := pidsToService[pid]; service != nil {
			continue
		}

		log.Debugf("no service found for pid: %d", pid)
		tries := c.serviceRetries[pid]
		tries++
		if tries < maxPortCheckTries {
			log.Debugf("adding service retry for pid: %d", pid)
			c.serviceRetries[pid] = tries
		} else {
			log.Tracef("[pid: %d] ignoring due to max number of retries", pid)
			c.ignoredPids.add(pid)
			delete(c.serviceRetries, pid)
		}
	}

	cleanPidMap(alivePids, c.ignoredPids)
	cleanPidMap(alivePids, c.serviceRetries)
}

func (c *collector) getPidsToRequest(alivePids pidSet) ([]int32, map[int32]*model.Service) {
	pidsToRequest := make([]int32, 0, len(alivePids))
	pidsToService := make(map[int32]*model.Service, len(alivePids))

	for pid := range alivePids {
		if c.ignoredPids.has(pid) {
			continue
		}

		// TODO: check heartbeat here

		pidsToRequest = append(pidsToRequest, pid)
		pidsToService[pid] = nil
	}

	return pidsToRequest, pidsToService
}

func (c *collector) getDiscoveryServices(pids []int32) (*model.ServicesResponse, error) {
	var responseData model.ServicesResponse

	url := getDiscoveryURL("services", pids)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.sysProbeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("non-ok status code: url %s, status_code: %d, response: `%s`", req.URL, resp.StatusCode, string(body))
	}

	err = json.Unmarshal(body, &responseData)
	if err != nil {
		return nil, err
	}

	return &responseData, nil
}

func getAlivePids() (pidSet, error) {
	pids, err := process.Pids()
	if err != nil {
		return nil, err
	}

	alivePids := make(pidSet)
	for _, pid := range pids {
		alivePids.add(pid)
	}

	return alivePids, nil
}

func getDiscoveryURL(endpoint string, pids []int32) string {
	URL := &url.URL{
		Scheme: "http",
		Host:   "sysprobe",
		Path:   "/discovery/" + endpoint,
	}

	if len(pids) > 0 {
		pidsStr := make([]string, len(pids))
		for i, pid := range pids {
			pidsStr[i] = strconv.Itoa(int(pid))
		}

		query := url.Values{}
		query.Add("pids", strings.Join(pidsStr, ","))
		URL.RawQuery = query.Encode()
	}

	return URL.String()
}
