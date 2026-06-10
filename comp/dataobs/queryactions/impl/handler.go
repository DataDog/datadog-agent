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
	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"
)

// activeConfigEntry stores the scheduled check config alongside the base postgres config
// metadata and parsed instance, so that disabling can restore the original config.
type activeConfigEntry struct {
	checkConfig integration.Config
	baseCfg     *integration.Config // the original matched postgres config for restoration
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

		// Validate each query spec before paying the cost of finding the postgres config.
		// On the first invalid query, reject the entire config — no partial scheduling.
		var validationErr error
		for _, q := range payload.Queries {
			if err := validateQuerySpec(q); err != nil {
				validationErr = err
				break
			}
		}
		if validationErr != nil {
			c.log.Warnf("Invalid DO query spec in config %s: %v", configID, validationErr)
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateError, Error: validationErr.Error()})
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

		// Remove previous DO config version if this config_id was already active.
		c.removeActiveConfig(configID, &changes)
		// Unschedule the base file-provider config to prevent duplicate check execution.
		// No-op in autodiscovery if the base config was already unscheduled by a prior update.
		changes.Unschedule = append(changes.Unschedule, *baseCfg)

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

// removeActiveConfig removes a config from activeConfigs and adds the previous DO check
// config to changes.Unschedule. Used before scheduling an updated DO config (where the
// base config should NOT be restored). It is a no-op if configID is not currently active.
func (c *component) removeActiveConfig(configID string, changes *integration.ConfigChanges) {
	c.activeConfigsMu.Lock()
	prev, existed := c.activeConfigs[configID]
	if existed {
		delete(c.activeConfigs, configID)
	}
	c.activeConfigsMu.Unlock()

	if !existed {
		return
	}

	changes.Unschedule = append(changes.Unschedule, prev.checkConfig)
}

// collectDisable removes a config from activeConfigs, unschedules the DO check config,
// and re-schedules the original base postgres config to restore normal check behavior.
// It is a no-op if configID is not currently active.
func (c *component) collectDisable(configID string, changes *integration.ConfigChanges) {
	c.activeConfigsMu.Lock()
	prev, existed := c.activeConfigs[configID]
	if existed {
		delete(c.activeConfigs, configID)
	}
	c.activeConfigsMu.Unlock()

	if !existed {
		return
	}

	changes.Unschedule = append(changes.Unschedule, prev.checkConfig)
	changes.Schedule = append(changes.Schedule, *prev.baseCfg)
	c.log.Infof("Disabled Data Observability query actions for config: %s", configID)
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
		return nil, nil, fmt.Errorf("no postgres config found for identifier: type=%s, host=%s; at least one postgres instance had a YAML parse error: %w",
			dbID.Type, dbID.Host, lastParseErr)
	}
	return nil, nil, fmt.Errorf("no postgres config found for identifier: type=%s, host=%s",
		dbID.Type, dbID.Host)
}

// matchesIdentifier checks if an instance matches the given DB identifier.
// Matching is by host only — per-query dbname fields handle database routing.
func matchesIdentifier(instance map[string]any, dbID *DBIdentifier) bool {
	host, _ := instance["host"].(string)
	return host == dbID.Host
}

// buildCheckConfig creates a postgres check config with data_observability queries injected.
// It clones the full matched postgres instance and adds the data_observability section.
// Returns an error if YAML serialization fails; callers must report ApplyStateError to RC.
func (c *component) buildCheckConfig(payload *DOQueryPayload, baseCfg *integration.Config, instance map[string]any, remoteConfigID string) (integration.Config, error) {
	queries := make([]map[string]any, 0, len(payload.Queries))
	for _, q := range payload.Queries {
		// Force the query string to be emitted as a double-quoted scalar.
		// yaml.v3's default style selection picks a `|N` block scalar for multi-line
		// strings, which is unparseable when the source string mixes indentation
		// across lines (e.g. a SQL body with -- comment trailing at column 0). A
		// double-quoted scalar preserves the content byte-for-byte and parses
		// unambiguously on the receiving side.
		queryNode := &yaml.Node{Kind: yaml.ScalarNode, Style: yaml.DoubleQuotedStyle, Value: q.Query}
		qm := map[string]any{
			"dbname":        q.DBName,
			"monitor_id":    q.MonitorID,
			"type":          q.Type,
			"query":         queryNode,
			"query_timeout": q.TimeoutSeconds * 1000,
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
		if q.IntervalSeconds > 0 {
			qm["interval_seconds"] = q.IntervalSeconds
		}
		if q.Schedule != "" {
			qm["schedule"] = q.Schedule
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

// validateQuerySpec validates a QuerySpec before scheduling.
// A query is valid iff exactly one of the following holds:
//   - schedule is non-empty and is a valid 5-field standard cron expression.
//   - schedule is empty and interval_seconds > 0.
//
// When both fields are set (schedule non-empty and interval_seconds > 0), the cron
// schedule takes precedence downstream; the query is still accepted here as valid.
func validateQuerySpec(q QuerySpec) error {
	if q.Schedule != "" {
		// Validate the cron expression using the same ParseStandard parser as
		// pkg/collector/corechecks/cluster/ksm/customresources/cronjob.go:382
		// to guarantee identical semantics between Go validation and Python scheduling.
		if _, err := cron.ParseStandard(q.Schedule); err != nil {
			return fmt.Errorf("monitor_id %d: invalid cron schedule %q: %w", q.MonitorID, q.Schedule, err)
		}
		return nil
	}
	if q.IntervalSeconds <= 0 {
		return fmt.Errorf("monitor_id %d: interval_seconds must be > 0 when schedule is unset", q.MonitorID)
	}
	return nil
}
