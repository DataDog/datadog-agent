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
// from an existing postgres instance config into a Data Observability query actions check.
// This list must never include "remote_config_id", "db_type", "db_identifier", or "queries" —
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
			c.collectUnschedule(configID, &changes)
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
			continue
		}

		auth, baseCfg, err := c.resolveCredentials(&payload.DBIdentifier)
		if err != nil {
			c.log.Warnf("No matching credentials for %s: %v", configID, err)
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			c.collectUnschedule(configID, &changes)
			continue
		}

		remoteConfigID := rawConfig.Metadata.ID
		if remoteConfigID == "" {
			remoteConfigID = configID
		}

		checkConfig, err := c.buildCheckConfig(&payload, baseCfg, auth, remoteConfigID)
		if err != nil {
			c.log.Errorf("Failed to build check config for %s: %v", configID, err)
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			c.collectUnschedule(configID, &changes)
			continue
		}

		// Unschedule previous version before scheduling updated config
		c.collectUnschedule(configID, &changes)

		c.activeConfigsMu.Lock()
		c.activeConfigs[configID] = checkConfig
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
		c.log.Infof("Config %s absent from RC snapshot, unscheduling", configID)
		c.collectUnschedule(configID, &changes)
	}

	return changes
}

// collectUnschedule removes a config from activeConfigs and appends it to changes.Unschedule.
// It is a no-op if configID is not currently active.
func (c *component) collectUnschedule(configID string, changes *integration.ConfigChanges) {
	c.activeConfigsMu.Lock()
	prev, existed := c.activeConfigs[configID]
	if existed {
		delete(c.activeConfigs, configID)
	}
	c.activeConfigsMu.Unlock()

	if !existed {
		return
	}

	changes.Unschedule = append(changes.Unschedule, prev)
	c.log.Infof("Unscheduled Data Observability query action check: %s", configID)
}

// resolveCredentials finds credentials for the given DB identifier.
// It first checks DO-specific database configs from datadog.yaml, then falls back
// to borrowing credentials from a matching postgres check config.
func (c *component) resolveCredentials(dbID *DBIdentifier) (map[string]any, *integration.Config, error) {
	// First: check dedicated DO database credentials
	for i := range c.databases {
		if c.databases[i].matchesIdentifier(dbID) {
			baseCfg := &integration.Config{
				Name:     "postgres",
				Provider: "datadog.yaml",
			}
			return c.databases[i].toInstanceMap(), baseCfg, nil
		}
	}

	// Fallback: borrow credentials from a matching postgres check instance
	baseCfg, instance, err := c.findPostgresConfig(dbID)
	if err != nil {
		return nil, nil, err
	}

	key := dbID.Host + ":" + dbID.DBName
	if !c.warnedFallback[key] {
		c.warnedFallback[key] = true
		c.log.Warnf("Using credentials from postgres check for DO query actions (host=%s, dbname=%s); "+
			"none of the %d entries in data_observability.query_actions.databases matched.",
			dbID.Host, dbID.DBName, len(c.databases))
	}

	return extractDBAuthFromInstance(instance), baseCfg, nil
}

// findPostgresConfig finds a postgres config that matches the given identifier.
// Returns the matching config and the already-parsed instance map to avoid re-parsing YAML in callers.
func (c *component) findPostgresConfig(dbID *DBIdentifier) (*integration.Config, map[string]any, error) {
	cfgs := c.ac.GetUnresolvedConfigs()

	var lastParseErr error
	for cfgIdx := range cfgs {
		cfg := cfgs[cfgIdx]
		if !isPostgresIntegration(cfg.Name) {
			continue
		}

		for _, instanceData := range cfg.Instances {
			var instance map[string]any
			if err := yaml.Unmarshal(instanceData, &instance); err != nil {
				c.log.Warnf("Failed to unmarshal postgres instance data for config %s, skipping: %v", cfg.Name, err)
				lastParseErr = err
				continue
			}

			if matchesIdentifier(instance, dbID) {
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

// extractDBAuthFromInstance extracts credential fields from a parsed instance map using an allowlist.
// The instance map comes from findPostgresConfig, which already parsed the YAML.
func extractDBAuthFromInstance(instance map[string]any) map[string]any {
	out := make(map[string]any)
	for _, k := range dbCredentialAllowList {
		if v, ok := instance[k]; ok {
			out[k] = v
		}
	}
	return out
}

// buildCheckConfig creates a check config for the do_query_actions check.
// auth contains the database credential fields already resolved by resolveCredentials.
// Returns an error if instance YAML serialization fails;
// callers must report ApplyStateError to RC rather than scheduling the broken config.
func (c *component) buildCheckConfig(payload *DOQueryPayload, baseCfg *integration.Config, auth map[string]any, remoteConfigID string) (integration.Config, error) {

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
		"db_identifier": map[string]any{
			"host":   payload.DBIdentifier.Host,
			"dbname": payload.DBIdentifier.DBName,
		},
		"queries": queries,
	}

	// Copy auth fields into instance config. Auth fields from dbCredentialAllowList
	// will overwrite any matching keys already set (none currently overlap).
	// dbCredentialAllowList must not include "remote_config_id", "db_type", "db_identifier", or "queries".
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
