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

// dbCredentialAllowList defines the connection and authentication fields to copy
// from an existing postgres instance config into a DO query actions check.
// This list must never include "remote_config_id", "db_type", or "queries" —
// those keys are set programmatically and auth fields copied via maps.Copy
// would silently overwrite them.
var dbCredentialAllowList = []string{
	"host", "port", "username", "password", "dbname",
	"ssl", "ssl_mode", "ssl_cert", "ssl_key", "ssl_root_cert",
	"tls", "tls_verify", "tls_cert", "tls_key", "tls_ca_cert",
	"aws", "managed_authentication",
}

// isPostgresIntegration reports whether name is "postgres". Only postgres is currently supported.
func isPostgresIntegration(name string) bool {
	return name == "postgres"
}

// onRCUpdate handles DO_QUERY_ACTIONS RC product updates with a declarative config model.
// The full updates map is treated as a snapshot: configs absent from the current update are
// unscheduled. An empty queries list signals removal of all queries for that config.
func (c *component) onRCUpdate(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) {
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
		c.log.Infof("Received DO query action config: %s (config_id: %s, queries: %d)", path, configID, len(payload.Queries))

		// Empty queries list signals all queries for this config should be removed
		if len(payload.Queries) == 0 {
			c.unscheduleConfig(configID)
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
			continue
		}

		baseCfg, instanceData, err := c.findPostgresConfig(&payload.DBIdentifier)
		if err != nil {
			c.log.Warnf("No matching postgres config for %s: %v", configID, err)
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}

		remoteConfigID := rawConfig.Metadata.ID
		if remoteConfigID == "" {
			remoteConfigID = configID
		}

		checkConfig, err := c.buildCheckConfig(&payload, baseCfg, instanceData, remoteConfigID)
		if err != nil {
			c.log.Errorf("Failed to build check config for %s: %v", configID, err)
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}

		// Unschedule previous version before scheduling updated config
		c.unscheduleConfig(configID)

		scheduled := false
		c.closeMu.RLock()
		if !c.closed {
			select {
			case c.configChanges <- integration.ConfigChanges{Schedule: []integration.Config{checkConfig}}:
				c.activeConfigsMu.Lock()
				c.activeConfigs[configID] = checkConfig
				c.activeConfigsMu.Unlock()
				c.log.Infof("Scheduled DO query action check: %s (%d queries)", configID, len(payload.Queries))
				scheduled = true
			default:
				c.log.Warnf("Config changes channel full, dropping schedule for %s", configID)
			}
		}
		c.closeMu.RUnlock()

		if scheduled {
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
		} else {
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateError, Error: "component stopped or channel full"})
		}
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
		c.log.Infof("Config %s absent from RC snapshot, unscheduling", configID)
		c.unscheduleConfig(configID)
	}
}

// unscheduleConfig removes a previously scheduled check config by config ID.
func (c *component) unscheduleConfig(configID string) {
	c.activeConfigsMu.Lock()
	prev, existed := c.activeConfigs[configID]
	if existed {
		delete(c.activeConfigs, configID)
	}
	c.activeConfigsMu.Unlock()

	if !existed {
		return
	}

	c.closeMu.RLock()
	if !c.closed {
		select {
		case c.configChanges <- integration.ConfigChanges{Unschedule: []integration.Config{prev}}:
			c.log.Infof("Unscheduled DO query action check: %s", configID)
		default:
			c.log.Warnf("Config changes channel full, dropping unschedule for %s", configID)
		}
	}
	c.closeMu.RUnlock()
}

// findPostgresConfig finds a postgres config that matches the given identifier.
func (c *component) findPostgresConfig(dbID *DBIdentifier) (*integration.Config, integration.Data, error) {
	cfgs := c.ac.GetUnresolvedConfigs()

	for cfgIdx := range cfgs {
		cfg := cfgs[cfgIdx]
		if !isPostgresIntegration(cfg.Name) {
			continue
		}

		for _, instanceData := range cfg.Instances {
			var instance map[string]any
			if err := yaml.Unmarshal(instanceData, &instance); err != nil {
				c.log.Warnf("Failed to unmarshal postgres instance data for config %s, skipping: %v", cfg.Name, err)
				continue
			}

			if matchesIdentifier(instance, dbID) {
				return &cfg, instanceData, nil
			}
		}
	}

	return nil, nil, fmt.Errorf("no postgres config found for identifier: type=%s, host=%s, dbname=%s",
		dbID.Type, dbID.Host, dbID.DBName)
}

// matchesDBName checks if an instance's dbname matches the RC identifier's dbname.
// If the RC specifies no dbname, any instance matches. If the instance has no dbname set, it also matches.
// Otherwise both must match exactly.
func matchesDBName(instance map[string]any, dbID *DBIdentifier) bool {
	if dbID.DBName == "" {
		return true
	}
	instanceDBName, _ := instance["dbname"].(string)
	if instanceDBName == "" {
		return true
	}
	return instanceDBName == dbID.DBName
}

// matchesIdentifier checks if an instance matches the given DB identifier.
// Only "self-hosted" type is supported; matching is by host and dbname.
func matchesIdentifier(instance map[string]any, dbID *DBIdentifier) bool {
	if dbID.Type != "self-hosted" {
		return false
	}
	host, _ := instance["host"].(string)
	return host == dbID.Host && matchesDBName(instance, dbID)
}

// extractDBAuthFromInstanceData extracts credential fields from raw instance YAML using an allowlist.
func extractDBAuthFromInstanceData(instanceData integration.Data) (map[string]any, error) {
	out := make(map[string]any)
	raw := map[string]any{}
	if err := yaml.Unmarshal(instanceData, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse instance YAML: %w", err)
	}

	for _, k := range dbCredentialAllowList {
		if v, ok := raw[k]; ok {
			out[k] = v
		}
	}
	return out, nil
}

// buildCheckConfig creates a check config for the do_query_actions check.
// Returns an error if auth extraction or instance YAML serialization fails;
// callers must report ApplyStateError to RC rather than scheduling the broken config.
func (c *component) buildCheckConfig(payload *DOQueryPayload, baseCfg *integration.Config, instanceData integration.Data, remoteConfigID string) (integration.Config, error) {
	auth, err := extractDBAuthFromInstanceData(instanceData)
	if err != nil {
		return integration.Config{}, fmt.Errorf("failed to extract auth from instance data: %w", err)
	}

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

	instanceFields := map[string]any{
		"remote_config_id": remoteConfigID,
		"db_type":          baseCfg.Name,
		"queries":          queries,
	}

	// Copy auth fields into instance config. Auth fields from dbCredentialAllowList
	// will overwrite any matching keys already set (none currently overlap).
	// dbCredentialAllowList must not include "remote_config_id", "db_type", or "queries".
	maps.Copy(instanceFields, auth)

	instanceYAML, err := yaml.Marshal(instanceFields)
	if err != nil {
		return integration.Config{}, fmt.Errorf("failed to marshal check instance: %w", err)
	}

	return integration.Config{
		Name:      "do_query_actions",
		Source:    c.String(),
		Provider:  baseCfg.Provider,
		NodeName:  baseCfg.NodeName,
		Instances: []integration.Data{instanceYAML},
	}, nil
}
