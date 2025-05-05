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
)

type collector struct {
	id      string
	catalog workloadmeta.AgentType
	store   workloadmeta.Component

	sysProbeClient *http.Client
	startTime      time.Time
	startupTimeout time.Duration
}

func NewCollector() (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &collector{
			id:             collectorID,
			catalog:        workloadmeta.NodeAgent,
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
				pids, err := process.Pids()
				if err != nil {
					log.Errorf("failed to get pids: %s", err)
					continue
				}

				services, err := c.getDiscoveryServices(pids)
				if err != nil {
					if time.Since(c.startTime) < c.startupTimeout {
						log.Warnf("service collector: system-probe not started yet: %v", err)
						continue
					}

					log.Errorf("failed to get services: %s", err)
					continue
				}

				log.Debugf("service collector: running services count: %d", services.RunningServicesCount)
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
