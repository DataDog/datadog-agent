// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discoverer

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

// discoveryService is the JSON payload sent to the integration when asking it
// to discover its config for a given service.
type discoveryService struct {
	ID    string          `json:"id"`
	Host  string          `json:"host"`
	Ports []discoveryPort `json:"ports"`
}

type discoveryPort struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
}

// discoveredConfig is the JSON shape returned by the integration.
type discoveredConfig struct {
	Instances               []json.RawMessage `json:"instances"`
	InitConfig              json.RawMessage   `json:"init_config"`
	MetricConfig            json.RawMessage   `json:"metric_config"`
	LogsConfig              json.RawMessage   `json:"logs"`
	IgnoreAutodiscoveryTags bool              `json:"ignore_autodiscovery_tags"`
	CheckTagCardinality     string            `json:"check_tag_cardinality"`
}

// marshalService builds the JSON payload sent to the integration for the
// given live service. Returns ("", false, nil) when the service has no
// usable host yet — typical during container startup, treated by callers as
// a transient failure that warrants a retry.
func marshalService(svc ServiceInfo) (string, bool, error) {
	hosts, err := svc.GetHosts()
	if err != nil {
		return "", false, nil
	}
	host, ok := pickHost(hosts)
	if !ok {
		return "", false, nil
	}
	exposed, err := svc.GetPorts()
	if err != nil {
		return "", false, fmt.Errorf("GetPorts: %w", err)
	}
	payload := discoveryService{
		ID:    svc.GetServiceID(),
		Host:  host,
		Ports: make([]discoveryPort, 0, len(exposed)),
	}
	for _, p := range exposed {
		payload.Ports = append(payload.Ports, discoveryPort{Number: p.Port, Name: p.Name})
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", false, fmt.Errorf("marshal: %w", err)
	}
	return string(b), true, nil
}

// pickHost applies the same fallback policy as %%host%%: single network →
// use it; multiple networks with a "bridge" → use bridge; otherwise no
// deterministic choice is possible and we return false.
func pickHost(hosts map[string]string) (string, bool) {
	if len(hosts) == 0 {
		return "", false
	}
	if len(hosts) == 1 {
		for _, ip := range hosts {
			return ip, true
		}
	}
	if ip, ok := hosts["bridge"]; ok {
		return ip, true
	}
	return "", false
}

// parseDiscoveryResult turns the raw JSON returned by ConfigDiscoverer into a
// slice of integration.Config. The configs returned here are not yet resolved
// through configresolver — the caller is expected to run them through the
// normal substitution + secret-decryption path before scheduling.
func parseDiscoveryResult(integrationName, resultJSON string) ([]integration.Config, error) {
	var raws []discoveredConfig
	if err := json.Unmarshal([]byte(resultJSON), &raws); err != nil {
		return nil, fmt.Errorf("decode discovery payload for %s: %w", integrationName, err)
	}
	if len(raws) == 0 {
		return nil, nil
	}
	configs := make([]integration.Config, 0, len(raws))
	for _, raw := range raws {
		initConfig := raw.InitConfig
		if len(initConfig) == 0 {
			initConfig = json.RawMessage("{}")
		}
		cfg := integration.Config{
			Name:                    integrationName,
			InitConfig:              integration.Data(initConfig),
			MetricConfig:            integration.Data(raw.MetricConfig),
			LogsConfig:              integration.Data(raw.LogsConfig),
			IgnoreAutodiscoveryTags: raw.IgnoreAutodiscoveryTags,
			CheckTagCardinality:     raw.CheckTagCardinality,
		}
		for _, inst := range raw.Instances {
			cfg.Instances = append(cfg.Instances, integration.Data(inst))
		}
		configs = append(configs, cfg)
	}
	return configs, nil
}
