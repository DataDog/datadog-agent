// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package datasecurityimpl

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	yaml "gopkg.in/yaml.v3"
)

const (
	// dataSecuritySection is the key of the integration instance section that
	// carries the per-instance opt-in (enabled: true).
	dataSecuritySection = "data_security"

	// datasecurityCheckName is the shared-library check that performs the scan.
	datasecurityCheckName = "datasecurity"
)

// rcPayload is the DEBUG RC payload this component understands:
//
//	{
//	  "tasks": [
//	    {
//	      "scanning_rules": [ { "id": "...", "name": "...", "regex": "..." }, ... ],
//	      "scan_data": { "postgres": { "query": "SELECT ...", "table": "..." } }
//	    }
//	  ]
//	}
type rcPayload struct {
	Type  string   `json:"type" yaml:"type"`
	Tasks []rcTask `json:"tasks" yaml:"tasks"`
}

type rcTask struct {
	ScanningRules []rcScanningRule `json:"scanning_rules" yaml:"scanning_rules"`
	ScanData      rcScanData       `json:"scan_data" yaml:"scan_data"`
}

type rcScanningRule struct {
	ID    string `json:"id" yaml:"id"`
	Name  string `json:"name" yaml:"name"`
	Regex string `json:"regex" yaml:"regex"`
}

type rcScanData struct {
	Postgres *rcPostgresScanData `json:"postgres" yaml:"postgres"`
}

type rcPostgresScanData struct {
	Query string `json:"query" yaml:"query"`
	Table string `json:"table" yaml:"table"`
}

// runtimeInstanceConfig is forwarded in-memory to the datasecurity shared-library
// check. It is never written to disk.
type runtimeInstanceConfig struct {
	MinCollectionInterval int             `yaml:"min_collection_interval"`
	TaskID                string          `yaml:"task_id"`
	ScanConfig            rcPayload       `yaml:"scan_config"`
	Backend               string          `yaml:"backend"`
	Postgres              *postgresConfig `yaml:"postgres,omitempty"`
}

type postgresConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Dbname   string `yaml:"dbname"`
}

// onUpdate is invoked by the RC client with the full set of active configs for
// the DEBUG product. Each eligible payload is scheduled as a one-shot
// datasecurity shared-library check via autodiscovery.
func (c *component) onUpdate(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) {
	if len(updates) == 0 {
		c.log.Debugf("datasecurity: RC DEBUG update with 0 active config(s)")
		return
	}

	paths := make([]string, 0, len(updates))
	for path := range updates {
		paths = append(paths, path)
	}
	c.log.Infof("datasecurity: RC DEBUG update with %d config(s): %v", len(updates), paths)

	changes := integration.ConfigChanges{}
	for path, rawConfig := range updates {
		var payload rcPayload
		if err := json.Unmarshal(rawConfig.Config, &payload); err != nil {
			c.log.Warnf("datasecurity: failed to unmarshal DEBUG config %s: %v", path, err)
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}

		c.log.Infof("datasecurity: parsed DEBUG config %s (tasks=%d type=%q)",
			configShortName(path), len(payload.Tasks), payload.Type)

		checkCfg, err := c.buildCheckConfig(path, payload)
		if err != nil {
			if errors.Is(err, errNotDataSecurityPayload) {
				c.log.Debugf("datasecurity: skipping DEBUG config %s: %v", configShortName(path), err)
				applyStatus(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
				continue
			}
			if errors.Is(err, errNoScanData) {
				c.log.Infof("datasecurity: no supported scan target in payload for %s, nothing to schedule", configShortName(path))
				applyStatus(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
				continue
			}
			c.log.Warnf("datasecurity: cannot schedule DEBUG config from %s: %v", path, err)
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}

		changes.Schedule = append(changes.Schedule, checkCfg)
		c.log.Infof("datasecurity: scheduled one-shot %q check for task %s",
			datasecurityCheckName, configShortName(path))
		applyStatus(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}

	if !changes.IsEmpty() {
		c.sendChanges(changes)
	}
}

var (
	errNotDataSecurityPayload = errors.New("not a data security scan payload")
	errNoScanData             = errors.New("no supported scan target in payload")
)

// configShortName returns the RC config name segment (e.g. test-aimene-data-security).
func configShortName(path string) string {
	const prefix = "datadog/2/DEBUG/"
	if len(path) > len(prefix) && path[:len(prefix)] == prefix {
		rest := path[len(prefix):]
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			return rest[:i]
		}
		return rest
	}
	return path
}

// buildCheckConfig builds an autodiscovery config for a one-shot datasecurity
// shared-library check from the RC payload and the matching integration credentials.
func (c *component) buildCheckConfig(path string, payload rcPayload) (integration.Config, error) {
	if len(payload.Tasks) == 0 {
		return integration.Config{}, fmt.Errorf("%w: no tasks", errNotDataSecurityPayload)
	}

	backend, err := detectBackend(payload)
	if err != nil {
		return integration.Config{}, err
	}

	hasRules := false
	ruleCount := 0
	for _, t := range payload.Tasks {
		ruleCount += len(t.ScanningRules)
		if len(t.ScanningRules) > 0 {
			hasRules = true
		}
	}
	c.log.Infof("datasecurity: scan payload %s has %d task(s), %d rule(s), backend=%s",
		configShortName(path), len(payload.Tasks), ruleCount, backend)

	if !hasRules {
		return integration.Config{}, errors.New("no scanning rules in payload")
	}

	if !c.cfg.GetBool("shared_library_check.enabled") {
		return integration.Config{}, errors.New("shared_library_check.enabled must be true to run datasecurity scans")
	}

	instance, baseCfg, ok := c.findIntegrationInstance(backend)
	if !ok {
		backendDef, _ := backendByKind(backend)
		return integration.Config{}, fmt.Errorf(
			"no %s instance with data_security.enabled: true found",
			backendDef.IntegrationName(),
		)
	}

	runtimeCfg := runtimeInstanceConfig{
		MinCollectionInterval: 0,
		TaskID:                path,
		ScanConfig:            payload,
		Backend:               string(backend),
	}
	if err := applyRuntimeConnection(backend, instance, &runtimeCfg); err != nil {
		return integration.Config{}, err
	}

	instanceYAML, err := yaml.Marshal(runtimeCfg)
	if err != nil {
		return integration.Config{}, fmt.Errorf("marshaling runtime instance config: %w", err)
	}

	return integration.Config{
		Name:       datasecurityCheckName,
		Source:     fmt.Sprintf("%s:%s", c.String(), configShortName(path)),
		Instances:  []integration.Data{instanceYAML},
		InitConfig: integration.Data("{}"),
		Provider:   baseCfg.Provider,
		NodeName:   baseCfg.NodeName,
	}, nil
}

// instanceDataSecurityEnabled reports whether a parsed instance has opted into
// data security via data_security.enabled: true.
func instanceDataSecurityEnabled(instance map[string]any) bool {
	ds, ok := instance[dataSecuritySection].(map[string]any)
	if !ok {
		return false
	}
	enabled, _ := ds["enabled"].(bool)
	return enabled
}

// stringField returns the first key present in the instance whose value is a
// string, or "" if none is found.
func stringField(instance map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := instance[k].(string); ok {
			return v
		}
	}
	return ""
}
