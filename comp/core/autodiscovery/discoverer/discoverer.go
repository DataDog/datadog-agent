// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discoverer

import (
	"context"
	"encoding/json"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const defaultFailureTTL = 30 * time.Second

type defaultDiscoverer struct {
	bridge     Bridge
	cache      *cache
	failureTTL time.Duration
}

func newDiscoverer(bridge Bridge) *defaultDiscoverer {
	return &defaultDiscoverer{
		bridge:     bridge,
		cache:      newCache(time.Now),
		failureTTL: defaultFailureTTL,
	}
}

// New constructs a Discoverer wrapping the given Bridge. Pass nil bridge in
// configurations where Python is unavailable (cluster agent today); resolution
// of templates with Discovery set will then fail-closed.
func New(bridge Bridge) Discoverer {
	if bridge == nil {
		return nil
	}
	return newDiscoverer(bridge)
}

// servicePayload is the JSON shape passed across the rtloader bridge.
type servicePayload struct {
	ID    string        `json:"id"`
	Host  string        `json:"host"`
	Ports []portPayload `json:"ports"`
}

type portPayload struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
}

func (d *defaultDiscoverer) Discover(_ context.Context, integrationName string, svc listeners.Service) (Result, bool) {
	svcID := svc.GetServiceID()
	if r, ok, hit := d.cache.get(svcID, integrationName); hit {
		return r, ok
	}

	host, ok := pickHost(svc)
	if !ok {
		log.Debugf("autodiscovery/discoverer: %s has no host, skipping", svcID)
		d.cache.putFailure(svcID, integrationName, d.failureTTL)
		return Result{}, false
	}
	exposed, err := svc.GetPorts()
	if err != nil {
		log.Debugf("autodiscovery/discoverer: %s GetPorts error: %v", svcID, err)
		d.cache.putFailure(svcID, integrationName, d.failureTTL)
		return Result{}, false
	}

	payload := servicePayload{ID: svcID, Host: host}
	for _, p := range exposed {
		payload.Ports = append(payload.Ports, portPayload{Number: p.Port, Name: p.Name})
	}
	body, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("autodiscovery/discoverer: marshal failed for %s: %v", svcID, err)
		d.cache.putFailure(svcID, integrationName, d.failureTTL)
		return Result{}, false
	}

	resJSON, err := d.bridge.RunDiscover(integrationName, string(body))
	if err != nil {
		log.Warnf("autodiscovery/discoverer: %s.discover() failed for %s: %v", integrationName, svcID, err)
		d.cache.putFailure(svcID, integrationName, d.failureTTL)
		return Result{}, false
	}
	if resJSON == "" || resJSON == "null" {
		d.cache.putFailure(svcID, integrationName, d.failureTTL)
		return Result{}, false
	}

	var instances []json.RawMessage
	if err := json.Unmarshal([]byte(resJSON), &instances); err != nil {
		log.Errorf("autodiscovery/discoverer: %s returned non-list JSON for %s: %v", integrationName, svcID, err)
		d.cache.putFailure(svcID, integrationName, d.failureTTL)
		return Result{}, false
	}
	if len(instances) == 0 {
		d.cache.putFailure(svcID, integrationName, d.failureTTL)
		return Result{}, false
	}

	configs := make([]integration.Config, 0, len(instances))
	for _, raw := range instances {
		configs = append(configs, integration.Config{
			Name:      integrationName,
			Instances: []integration.Data{integration.Data(raw)},
		})
	}
	r := Result{Configs: configs}
	d.cache.putSuccess(svcID, integrationName, r)
	return r, true
}

func pickHost(svc listeners.Service) (string, bool) {
	hosts, err := svc.GetHosts()
	if err != nil || len(hosts) == 0 {
		return "", false
	}
	if h, ok := hosts["bridge"]; ok && h != "" {
		return h, true
	}
	for _, h := range hosts {
		if h != "" {
			return h, true
		}
	}
	return "", false
}
