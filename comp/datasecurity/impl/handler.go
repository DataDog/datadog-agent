// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package datasecurityimpl

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	yaml "gopkg.in/yaml.v3"
)

const (
	// postgresIntegrationName is the integration whose instance we look up to
	// run the scan query. Only postgres is supported for now.
	postgresIntegrationName = "postgres"

	// dataSecuritySection is the key of the postgres instance section that
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
//	      "scan_data": { "postgres": { "query": "SELECT ..." } }
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
	MinCollectionInterval int            `yaml:"min_collection_interval"`
	TaskID                string         `yaml:"task_id"`
	ScanConfig            rcPayload      `yaml:"scan_config"`
	Postgres              postgresConfig `yaml:"postgres"`
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
			if errors.Is(err, errNoPostgresQuery) {
				c.log.Infof("datasecurity: no postgres query in payload for %s, nothing to schedule", configShortName(path))
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
	errNoPostgresQuery        = errors.New("no postgres query in payload")
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
// shared-library check from the RC payload and the postgres instance credentials.
func (c *component) buildCheckConfig(path string, payload rcPayload) (integration.Config, error) {
	if len(payload.Tasks) == 0 {
		return integration.Config{}, fmt.Errorf("%w: no tasks", errNotDataSecurityPayload)
	}

	hasRules := false
	hasQuery := false
	ruleCount := 0
	var queryPreview string
	for _, t := range payload.Tasks {
		ruleCount += len(t.ScanningRules)
		if len(t.ScanningRules) > 0 {
			hasRules = true
		}
		if t.ScanData.Postgres != nil && t.ScanData.Postgres.Query != "" {
			hasQuery = true
			queryPreview = t.ScanData.Postgres.Query
		}
	}
	c.log.Infof("datasecurity: scan payload %s has %d task(s), %d rule(s), postgres_query=%t",
		configShortName(path), len(payload.Tasks), ruleCount, hasQuery)
	if hasQuery {
		c.log.Infof("datasecurity: postgres query for %s: %s", configShortName(path), queryPreview)
	}

	if !hasRules {
		return integration.Config{}, errors.New("no scanning rules in payload")
	}
	if !hasQuery {
		return integration.Config{}, errNoPostgresQuery
	}

	if !c.cfg.GetBool("shared_library_check.enabled") {
		return integration.Config{}, errors.New("shared_library_check.enabled must be true to run datasecurity scans")
	}

	instance, baseCfg, ok := c.findPostgresInstance()
	if !ok {
		return integration.Config{}, errors.New("no postgres instance with data_security.enabled: true found")
	}

	host := stringField(instance, "host")
	if host == "" {
		host = "localhost"
	}
	dbname := stringField(instance, "dbname", "database")
	user := stringField(instance, "username", "user")
	password := stringField(instance, "password")

	runtimeCfg := runtimeInstanceConfig{
		MinCollectionInterval: 0,
		TaskID:                path,
		ScanConfig:            payload,
		Postgres: postgresConfig{
			Host:     host,
			Port:     portInt(instance),
			Username: user,
			Password: password,
			Dbname:   dbname,
		},
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

// findPostgresInstance returns the first postgres instance with data_security.enabled: true
// and the integration config it belongs to.
// TODO: match against the database reported_hostname from the RC payload instead of
// taking the first eligible instance.
func (c *component) findPostgresInstance() (map[string]any, integration.Config, bool) {
	for _, cfg := range c.ac.GetUnresolvedConfigs() {
		if cfg.Name != postgresIntegrationName {
			continue
		}
		for _, instanceData := range cfg.Instances {
			var instance map[string]any
			if err := yaml.Unmarshal(instanceData, &instance); err != nil {
				continue
			}
			if !instanceDataSecurityEnabled(instance) {
				continue
			}

			c.log.Infof("datasecurity: using postgres instance (connect_host=%q reported_hostname=%q)",
				stringField(instance, "host"), stringField(instance, "reported_hostname"))
			return instance, cfg, true
		}
	}

	return nil, integration.Config{}, false
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

// portInt returns the postgres port as an int, defaulting to 5432.
func portInt(instance map[string]any) int {
	switch v := instance["port"].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		if port, err := strconv.Atoi(v); err == nil {
			return port
		}
	}
	return 5432
}
