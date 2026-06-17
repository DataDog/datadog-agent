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

// A "base config" is a supported DB integration.Config emitted by another provider (typically the
// file provider reading e.g. conf.d/postgres.d/conf.yaml or conf.d/sqlserver.d/conf.yaml) that a
// DO query action matched against via findSupportedIntegrationConfig — i.e. the config as it
// exists before DO touches it. A single base config can bundle several instances. Throughout this
// file, "base config" always refers to this original, provider-emitted config, as distinct from
// the DO check config or remainder config that this component derives from it.

// activeConfigEntry stores the scheduled DO check config alongside the base integration config it
// was derived from and the host it targets, so reconcileBases can rebuild the set of integration
// instances that should keep running independently of any single DO config.
type activeConfigEntry struct {
	checkConfig integration.Config
	baseCfg     *integration.Config // the original matched integration config (full, all instances)
	matchHost   string              // host this DO config targets (DBIdentifier.Host)
}

// managedBaseEntry tracks a base integration config that has at least one instance targeted by a
// DO query action. A DO query action only injects data_observability.queries into the targeted
// instance — every other field, and every other instance, is unchanged. But autodiscovery
// schedules whole configs (by digest), not single instances, so we cannot patch one instance in
// place: we unschedule the base config and schedule the targeted instance (with queries) plus a
// "remainder" config holding the base config's other instances verbatim. The original base config
// is retained here so it can be restored once no DO query action targets any of its instances.
type managedBaseEntry struct {
	original  integration.Config  // the full original base config, for restoration
	remainder *integration.Config // remainder config currently scheduled, or nil if none
}

// isSupportedIntegration reports whether name is a supported DB integration.
func isSupportedIntegration(name string) bool {
	return name == "postgres" || name == "sap_hana" || name == "sqlserver"
}

// instanceHost returns the host/server field for an integration instance,
// handling the fact that SAP HANA uses "server" while other integrations use "host".
func instanceHost(instance map[string]any) string {
	if host, ok := instance["host"].(string); ok && host != "" {
		return host
	}
	server, _ := instance["server"].(string)
	return server
}

// azureSQLDatabase returns the database when the instance uses Azure SQL Database.
func azureSQLDatabase(instance map[string]any) (string, bool) {
	azure, ok := instance["azure"].(map[string]any)
	if !ok {
		return "", false
	}
	deploymentType, _ := azure["deployment_type"].(string)
	if deploymentType != "sql_database" {
		return "", false
	}
	database, _ := instance["database"].(string)
	return database, true
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
			c.removeActiveConfig(configID, &changes)
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
			continue
		}

		// Validate each query spec before paying the cost of finding the integration config.
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

		baseCfg, instance, err := c.findSupportedIntegrationConfig(&payload.DBIdentifier)
		if err != nil {
			c.log.Warnf("No matching integration config for %s: %v", configID, err)
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			c.removeActiveConfig(configID, &changes)
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
			c.removeActiveConfig(configID, &changes)
			continue
		}

		// Remove previous DO config version if this config_id was already active.
		c.removeActiveConfig(configID, &changes)

		c.activeConfigsMu.Lock()
		c.activeConfigs[configID] = activeConfigEntry{
			checkConfig: checkConfig,
			baseCfg:     baseCfg,
			matchHost:   payload.DBIdentifier.Host,
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
		c.removeActiveConfig(configID, &changes)
	}

	// Reconcile base postgres configs: schedule remainder configs for partially-managed bases
	// and restore originals for bases no longer targeted by any DO config.
	c.reconcileBases(&changes)

	return changes
}

// removeActiveConfig removes a DO config from activeConfigs and adds its check config to
// changes.Unschedule. It does NOT touch the base config — base-config lifecycle (restoring
// the original file-provider config or its remainder) is owned by reconcileBases, which runs
// after all activeConfigs mutations for an update. No-op if configID is not currently active.
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

// reconcileBases keeps file-provider integration instances that are NOT targeted by a DO query
// action scheduled, while preventing the targeted instances from running twice.
//
// Autodiscovery schedules whole integration.Configs (keyed by Digest), but a single
// file-provider config can bundle several instances. When a DO config targets one of them, we
// cannot simply unschedule the whole base config — that would drop the untargeted sibling
// instances. Instead, for each base config that currently has at least one active DO config, we
// unschedule the original and schedule a "remainder" config holding only the instances no DO
// config targets. Once no DO config targets a base config, the original is restored.
//
// The remainder is computed from the full set of active DO configs, so multiple DO configs
// targeting different instances of the same base config never cause an instance to be both kept
// in the remainder and run as a DO check (which would duplicate DBM collection).
func (c *component) reconcileBases(changes *integration.ConfigChanges) {
	c.activeConfigsMu.Lock()
	defer c.activeConfigsMu.Unlock()

	// Group the hosts targeted by active DO configs per base config digest.
	type baseGroup struct {
		original integration.Config
		hosts    map[string]bool
	}
	desired := make(map[string]*baseGroup)
	for _, entry := range c.activeConfigs {
		digest := entry.baseCfg.Digest()
		g := desired[digest]
		if g == nil {
			g = &baseGroup{original: *entry.baseCfg, hosts: make(map[string]bool)}
			desired[digest] = g
		}
		g.hosts[entry.matchHost] = true
	}

	// Newly-managed or changed bases.
	for digest, g := range desired {
		remainder := buildRemainder(&g.original, g.hosts)
		managed, exists := c.managedBases[digest]
		if !exists {
			// First DO config to target this base: unschedule the original, schedule the remainder.
			changes.Unschedule = append(changes.Unschedule, g.original)
			if remainder != nil {
				changes.Schedule = append(changes.Schedule, *remainder)
			}
			c.managedBases[digest] = &managedBaseEntry{original: g.original, remainder: remainder}
			continue
		}
		// Already managed (original already unscheduled). Only touch the remainder if it changed,
		// to avoid needlessly restarting the untargeted instances.
		if sameConfig(managed.remainder, remainder) {
			continue
		}
		if managed.remainder != nil {
			changes.Unschedule = append(changes.Unschedule, *managed.remainder)
		}
		if remainder != nil {
			changes.Schedule = append(changes.Schedule, *remainder)
		}
		managed.remainder = remainder
	}

	// Bases no longer targeted by any DO config: unschedule the remainder, restore the original.
	for digest, managed := range c.managedBases {
		if _, ok := desired[digest]; ok {
			continue
		}
		if managed.remainder != nil {
			changes.Unschedule = append(changes.Unschedule, *managed.remainder)
		}
		changes.Schedule = append(changes.Schedule, managed.original)
		delete(c.managedBases, digest)
		c.log.Infof("Restored original integration config (digest %s); no Data Observability query actions target it", digest)
	}
}

// buildRemainder returns a copy of base containing only the instances whose host is NOT in
// matchedHosts. Returns nil when no instances remain (every instance is DO-managed). Instances
// whose YAML cannot be parsed are kept, so a config we cannot classify is never silently dropped.
func buildRemainder(base *integration.Config, matchedHosts map[string]bool) *integration.Config {
	kept := make([]integration.Data, 0, len(base.Instances))
	for _, instanceData := range base.Instances {
		var instance map[string]any
		if err := yaml.Unmarshal(instanceData, &instance); err != nil {
			kept = append(kept, instanceData)
			continue
		}
		host, _ := instance["host"].(string)
		if matchedHosts[host] {
			continue
		}
		kept = append(kept, instanceData)
	}
	if len(kept) == 0 {
		return nil
	}
	remainder := *base
	remainder.Instances = kept
	return &remainder
}

// sameConfig reports whether two optional configs are equivalent by autodiscovery digest.
func sameConfig(a, b *integration.Config) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Digest() == b.Digest()
}

// findSupportedIntegrationConfig finds a supported DB integration config that matches the
// given identifier and has data_observability.enabled: true. Returns the matching config
// and the already-parsed instance map to avoid re-parsing YAML in callers.
func (c *component) findSupportedIntegrationConfig(dbID *DBIdentifier) (*integration.Config, map[string]any, error) {
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
				c.log.Warnf("Failed to unmarshal %s instance data for config %s, skipping: %v", cfg.Name, cfg.Name, err)
				lastParseErr = err
				continue
			}

			if matchesIdentifier(instance, dbID) && instanceHasDOEnabled(instance) {
				return &cfg, instance, nil
			}
		}
	}

	if lastParseErr != nil {
		// Surface the parse error so operators debug the integration config YAML, not the RC identifier.
		return nil, nil, fmt.Errorf("no supported integration config found for identifier: type=%s, host=%s; at least one instance had a YAML parse error: %w",
			dbID.Type, dbID.Host, lastParseErr)
	}
	return nil, nil, fmt.Errorf("no supported integration config found for identifier: type=%s, host=%s",
		dbID.Type, dbID.Host)
}

// matchesIdentifier checks if an instance matches the given DB identifier.
// Most deployments match by host. SAP HANA also supports a server and port identifier.
// Azure SQL Database must match both host and database because databases share a server host.
func matchesIdentifier(instance map[string]any, dbID *DBIdentifier) bool {
	host := instanceHost(instance)
	hostMatches := host == dbID.Host
	if !hostMatches {
		if port, ok := instancePort(instance); ok {
			hostMatches = fmt.Sprintf("%s:%d", host, port) == dbID.Host
		}
	}
	if !hostMatches {
		return false
	}

	database, isAzureSQLDatabase := azureSQLDatabase(instance)
	return !isAzureSQLDatabase || database == dbID.Database
}

// instancePort returns the port number for an integration instance, if present.
func instancePort(instance map[string]any) (int, bool) {
	switch v := instance["port"].(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	}
	return 0, false
}

// buildCheckConfig creates a check config with data_observability queries injected.
// It clones the full matched instance and adds the data_observability section.
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
		Name:      baseCfg.Name,
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
