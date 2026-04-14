// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package queryactionsimpl

import (
	"encoding/json"
	"fmt"
	"maps"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"gopkg.in/yaml.v3"
)

// activeConfigEntry stores the scheduled check config alongside the base postgres config
// metadata and parsed instance, so that collectDisable can rebuild a "disable" config.
type activeConfigEntry struct {
	checkConfig integration.Config
	baseCfg     *integration.Config // Provider, NodeName from the matched postgres config
	instance    map[string]any      // full parsed postgres instance for rebuilding
}

// isSupportedIntegration reports whether name is a supported DB integration.
// Currently only postgres is supported; mysql may be added in the future.
func isSupportedIntegration(name string) bool {
	return name == "postgres"
}

// instanceHasDOEnabled checks whether a parsed instance map has data_observability.enabled: true.
func instanceHasDOEnabled(instance map[string]any) bool {
	doSection, ok := instance["data_observability"].(map[string]any)
	if !ok {
		return false
	}
	enabled, _ := doSection["enabled"].(bool)
	return enabled
}

// onRCUpdate handles DO_QUERY_ACTIONS RC product updates with a declarative config model.
// The full updates map is treated as a snapshot: configs absent from the current update are
// unscheduled. An empty queries list signals removal of all queries for that config.
// All schedule/unschedule changes are collected into a single returned ConfigChanges.
// The caller is responsible for delivering changes to autodiscovery.
func (c *component) onRCUpdate(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) integration.ConfigChanges {
	changes := integration.ConfigChanges{}
	seenConfigIDs := make(map[string]bool, len(updates))

	for path, rawConfig := range updates {
		var payload DOQueryPayload
		if err := json.Unmarshal(rawConfig.Config, &payload); err != nil {
			c.log.Warnf("Failed to unmarshal DO_QUERY_ACTIONS config %s: %v", path, err)
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}

		configID := payload.ConfigID
		if configID == "" {
			c.log.Errorf("DO query action config %s has empty config_id, skipping", path)
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateError, Error: "empty config_id"})
			continue
		}

		seenConfigIDs[configID] = true
		c.log.Debugf("Received DO query action config: %s (config_id: %s, queries: %d)", path, configID, len(payload.Queries))

		// Empty queries list signals all queries for this config should be removed
		if len(payload.Queries) == 0 {
			c.collectDisable(configID, &changes)
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
			continue
		}

		baseCfg, instance, err := c.findPostgresConfig(&payload.DBIdentifier)
		if err != nil {
			c.log.Warnf("No matching postgres config for %s: %v", configID, err)
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			c.collectDisable(configID, &changes)
			continue
		}

		remoteConfigID := rawConfig.Metadata.ID
		if remoteConfigID == "" {
			remoteConfigID = configID
		}

		checkConfig, err := c.buildCheckConfig(&payload, baseCfg, instance, remoteConfigID)
		if err != nil {
			c.log.Errorf("Failed to build check config for %s: %v", configID, err)
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			c.collectDisable(configID, &changes)
			continue
		}

		// Disable previous version before scheduling updated config
		c.collectDisable(configID, &changes)

		c.activeConfigsMu.Lock()
		c.activeConfigs[configID] = activeConfigEntry{
			checkConfig: checkConfig,
			baseCfg:     baseCfg,
			instance:    instance,
		}
		c.activeConfigsMu.Unlock()
		changes.Schedule = append(changes.Schedule, checkConfig)
		c.log.Infof("Scheduled Data Observability query action check: %s (%d queries)", configID, len(payload.Queries))
		applyStatus(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}

	// Reconcile: unschedule previously active configs absent from this snapshot
	c.activeConfigsMu.Lock()
	var toUnschedule []string
	for configID := range c.activeConfigs {
		if !seenConfigIDs[configID] {
			toUnschedule = append(toUnschedule, configID)
		}
	}
	c.activeConfigsMu.Unlock()

	for _, configID := range toUnschedule {
		c.log.Infof("Config %s absent from RC snapshot, disabling", configID)
		c.collectDisable(configID, &changes)
	}

	return changes
}

// collectDisable removes a config from activeConfigs and replaces it with a disable
// config that turns off data_observability on the postgres instance. The previous
// enabled config is unscheduled so autodiscovery removes the old YAML variant, and
// a new config with data_observability.enabled: false is scheduled in its place.
// It is a no-op if configID is not currently active.
func (c *component) collectDisable(configID string, changes *integration.ConfigChanges) {
	c.activeConfigsMu.Lock()
	prev, existed := c.activeConfigs[configID]
	c.activeConfigsMu.Unlock()

	if !existed {
		return
	}

	disableCfg, err := c.buildDisableConfig(prev.baseCfg, prev.instance)
	if err != nil {
		// Don't delete from activeConfigs — the old config stays tracked so a
		// future reconciliation can retry the disable.
		c.log.Errorf("Failed to build disable config for %s: %v", configID, err)
		return
	}

	// Only delete after successfully building the disable config.
	c.activeConfigsMu.Lock()
	delete(c.activeConfigs, configID)
	c.activeConfigsMu.Unlock()

	// Unschedule the previous enabled config so autodiscovery removes the old
	// YAML variant (different FastDigest), then schedule the disable config.
	changes.Unschedule = append(changes.Unschedule, prev.checkConfig)
	changes.Schedule = append(changes.Schedule, disableCfg)
	c.log.Infof("Disabled Data Observability query actions for config: %s", configID)
}

// buildDisableConfig creates a postgres config with data_observability.enabled: false.
func (c *component) buildDisableConfig(baseCfg *integration.Config, instance map[string]any) (integration.Config, error) {
	instanceFields := maps.Clone(instance)
	instanceFields["data_observability"] = map[string]any{
		"enabled": false,
	}

	instanceYAML, err := yaml.Marshal(instanceFields)
	if err != nil {
		return integration.Config{}, fmt.Errorf("failed to marshal disable instance: %w", err)
	}

	return integration.Config{
		Name:      "postgres",
		Source:    c.String(),
		Provider:  baseCfg.Provider,
		NodeName:  baseCfg.NodeName,
		Instances: []integration.Data{instanceYAML},
	}, nil
}

// findPostgresConfig finds a postgres config that matches the given identifier and has
// data_observability.enabled: true. Returns the matching config and the already-parsed
// instance map to avoid re-parsing YAML in callers.
func (c *component) findPostgresConfig(dbID *DBIdentifier) (*integration.Config, map[string]any, error) {
	cfgs := c.ac.GetUnresolvedConfigs()

	var lastParseErr error
	for cfgIdx := range cfgs {
		cfg := cfgs[cfgIdx]
		if !isSupportedIntegration(cfg.Name) {
			continue
		}

		for _, instanceData := range cfg.Instances {
			var instance map[string]any
			if err := yaml.Unmarshal(instanceData, &instance); err != nil {
				c.log.Warnf("Failed to unmarshal postgres instance data for config %s, skipping: %v", cfg.Name, err)
				lastParseErr = err
				continue
			}

			if matchesIdentifier(instance, dbID) && instanceHasDOEnabled(instance) {
				return &cfg, instance, nil
			}
		}
	}

	if lastParseErr != nil {
		// Surface the parse error so operators debug the postgres config YAML, not the RC identifier.
		return nil, nil, fmt.Errorf("no postgres config found for identifier: type=%s, host=%s, dbname=%s; at least one postgres instance had a YAML parse error: %w",
			dbID.Type, dbID.Host, dbID.DBName, lastParseErr)
	}
	return nil, nil, fmt.Errorf("no postgres config found for identifier: type=%s, host=%s, dbname=%s",
		dbID.Type, dbID.Host, dbID.DBName)
}

// matchesDBName checks if an instance's dbname matches the RC identifier's dbname exactly.
// Both must be equal; an empty RC dbname only matches instances that also have no dbname configured.
func matchesDBName(instance map[string]any, dbID *DBIdentifier) bool {
	instanceDBName, _ := instance["dbname"].(string)
	return instanceDBName == dbID.DBName
}

// matchesIdentifier checks if an instance matches the given DB identifier.
// Matching is by host and dbname, regardless of hosting type.
func matchesIdentifier(instance map[string]any, dbID *DBIdentifier) bool {
	host, _ := instance["host"].(string)
	return host == dbID.Host && matchesDBName(instance, dbID)
}

// buildCheckConfig creates a postgres check config with data_observability queries injected.
// It clones the full matched postgres instance and adds the data_observability section.
// Returns an error if YAML serialization fails; callers must report ApplyStateError to RC.
func (c *component) buildCheckConfig(payload *DOQueryPayload, baseCfg *integration.Config, instance map[string]any, remoteConfigID string) (integration.Config, error) {
	queries := make([]map[string]any, 0, len(payload.Queries))
	for _, q := range payload.Queries {
		qm := map[string]any{
			"monitor_id":       q.MonitorID,
			"type":             q.Type,
			"query":            q.Query,
			"interval_seconds": q.IntervalSeconds,
			"timeout_seconds":  q.TimeoutSeconds,
			"entity": map[string]any{
				"platform": q.Entity.Platform,
				"account":  q.Entity.Account,
				"database": q.Entity.Database,
				"schema":   q.Entity.Schema,
				"table":    q.Entity.Table,
			},
		}
		if q.CustomSQLSelectFields != nil {
			qm["custom_sql_select_fields"] = map[string]any{
				"metric_config_id": q.CustomSQLSelectFields.MetricConfigID,
				"entity_id":        q.CustomSQLSelectFields.EntityID,
			}
		}
		queries = append(queries, qm)
	}

	instanceFields := maps.Clone(instance)
	instanceFields["data_observability"] = map[string]any{
		"enabled":             true,
		"collection_interval": 10,
		"config_id":           remoteConfigID,
		"queries":             queries,
	}

	instanceYAML, err := yaml.Marshal(instanceFields)
	if err != nil {
		return integration.Config{}, fmt.Errorf("failed to marshal check instance: %w", err)
	}

	return integration.Config{
		Name:      "postgres",
		Source:    c.String(),
		Provider:  baseCfg.Provider,
		NodeName:  baseCfg.NodeName,
		Instances: []integration.Data{instanceYAML},
	}, nil
}
