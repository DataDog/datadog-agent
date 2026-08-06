// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package queryactionsimpl

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"
)

var databaseIdentifierVariablePattern = regexp.MustCompile(`\$\$|\$\{[A-Za-z_][A-Za-z0-9_]*\}|\$[A-Za-z_][A-Za-z0-9_]*`)

const defaultPostgresPort = 5432

// A "base config" is a postgres integration.Config emitted by another provider (typically the
// file provider reading conf.d/postgres.d/conf.yaml) that a DO query action matched against via
// findPostgresConfig — i.e. the config as it exists before DO touches it. A single base config
// can bundle several postgres instances. Throughout this file, "base config" always refers to
// this original, provider-emitted config, as distinct from the DO check config or remainder
// config that this component derives from it.

// activeConfigEntry stores the scheduled DO check config alongside the base postgres config it
// was derived from and the instance identity it targets, so reconcileBases can rebuild the set of
// postgres instances that should keep running independently of any single DO config.
type activeConfigEntry struct {
	checkConfig   integration.Config
	baseCfg       *integration.Config    // the original matched postgres config (full, all instances)
	matchInstance instanceConfigIdentity // exact source instance this DO config targets
}

// instanceConfigIdentity identifies the exact integration.Data entry selected from a base
// config. Endpoint-derived identities are not sufficient: two instances may share host and port
// while using different database_identifier templates or tags.
type instanceConfigIdentity [sha256.Size]byte

// managedBaseEntry tracks a base postgres config that has at least one instance targeted by a
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

// instanceHost returns the host/server field for an integration instance,
// handling the fact that sap_hana uses "server" while postgres uses "host".
func instanceHost(instance map[string]any) string {
	if host, ok := instance["host"].(string); ok && host != "" {
		return host
	}
	server, _ := instance["server"].(string)
	return server
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

		baseCfg, instance, matchInstance, err := c.resolveBaseConfig(configID, &payload.DBIdentifier)
		if err != nil {
			c.log.Warnf("No matching postgres config for %s: %v", configID, err)
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
			checkConfig:   checkConfig,
			baseCfg:       baseCfg,
			matchInstance: matchInstance,
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

// reconcileBases keeps file-provider postgres instances that are NOT targeted by a DO query
// action scheduled, while preventing the targeted instances from running twice.
//
// Autodiscovery schedules whole integration.Configs (keyed by Digest), but a single
// file-provider postgres config can bundle several instances. When a DO config targets one of
// them, we cannot simply unschedule the whole base config — that would drop the untargeted
// sibling instances. Instead, for each base config that currently has at least one active DO
// config, we unschedule the original and schedule a "remainder" config holding only the
// instances no DO config targets. Once no DO config targets a base config, the original is
// restored.
//
// The remainder is computed from the full set of active DO configs, so multiple DO configs
// targeting different instances of the same base config never cause an instance to be both
// kept in the remainder and run as a DO check (which would duplicate DBM collection).
func (c *component) reconcileBases(changes *integration.ConfigChanges) {
	c.activeConfigsMu.Lock()
	defer c.activeConfigsMu.Unlock()

	// Group the exact source instances targeted by active DO configs per base config digest.
	type baseGroup struct {
		original  integration.Config
		instances map[instanceConfigIdentity]bool
	}
	desired := make(map[string]*baseGroup)
	for _, entry := range c.activeConfigs {
		digest := entry.baseCfg.Digest()
		g := desired[digest]
		if g == nil {
			g = &baseGroup{original: *entry.baseCfg, instances: make(map[instanceConfigIdentity]bool)}
			desired[digest] = g
		}
		g.instances[entry.matchInstance] = true
	}

	// Newly-managed or changed bases.
	for digest, g := range desired {
		remainder := buildRemainder(&g.original, g.instances)
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
		c.log.Infof("Restored original postgres config (digest %s); no Data Observability query actions target it", digest)
	}
}

// buildRemainder returns a copy of base containing only instances that were not selected by a DO
// config. It returns nil when every instance is DO-managed. Matching the exact source YAML entry
// preserves siblings even when they share an endpoint but render different database identifiers.
func buildRemainder(base *integration.Config, matchedInstances map[instanceConfigIdentity]bool) *integration.Config {
	kept := make([]integration.Data, 0, len(base.Instances))
	for _, instanceData := range base.Instances {
		if matchedInstances[identifyInstanceConfig(instanceData)] {
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

func identifyInstanceConfig(instanceData integration.Data) instanceConfigIdentity {
	return sha256.Sum256(instanceData)
}

// sameConfig reports whether two optional configs are equivalent by autodiscovery digest.
func sameConfig(a, b *integration.Config) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Digest() == b.Digest()
}

// resolveBaseConfig returns the base config a DO check for configID should be derived from.
//
// If configID is already active, its previously-resolved base is reused as-is rather than
// re-derived from the current active set. Once reconcileBases unschedules a base config's
// targeted instance in favor of a DO check, that instance is no longer present in
// GetUnresolvedConfigs() (autodiscovery only reports currently-scheduled configs) — so a fresh
// findMatchingConfig search would instead match the DO component's own previously-scheduled check
// output, which also satisfies matchesIdentifier and instanceHasDOEnabled. Adopting that as the
// new "base" corrupts the digest reconcileBases uses to track the true original, causing it to be
// wrongly restored on this update. Reusing the stored base avoids re-deriving it from a set that
// may no longer contain it.
//
// Falls back to a fresh search if the stored base no longer has an instance matching dbID (e.g.
// its host genuinely changed between updates).
func (c *component) resolveBaseConfig(configID string, dbID *DBIdentifier) (*integration.Config, map[string]any, instanceConfigIdentity, error) {
	c.activeConfigsMu.Lock()
	existing, alreadyActive := c.activeConfigs[configID]
	c.activeConfigsMu.Unlock()

	if alreadyActive {
		instance, identity, err := c.findMatchingInstance(existing.baseCfg, dbID)
		if instance != nil {
			return existing.baseCfg, instance, identity, nil
		}
		if err != nil {
			c.log.Warnf("Stored base config for %s no longer parses cleanly, re-resolving: %v", configID, err)
		}
	}
	return c.findMatchingConfig(dbID)
}

// findMatchingConfig finds a supported DB integration config that matches the given identifier
// and has data_observability.enabled: true. Returns the matching config and the already-parsed
// instance map to avoid re-parsing YAML in callers.
func (c *component) findMatchingConfig(dbID *DBIdentifier) (*integration.Config, map[string]any, instanceConfigIdentity, error) {
	cfgs := c.ac.GetUnresolvedConfigs()

	var lastParseErr error
	for cfgIdx := range cfgs {
		cfg := cfgs[cfgIdx]
		instance, identity, err := c.findMatchingInstance(&cfg, dbID)
		if err != nil {
			lastParseErr = err
		}
		if instance != nil {
			return &cfg, instance, identity, nil
		}
	}

	if lastParseErr != nil {
		return nil, nil, instanceConfigIdentity{}, fmt.Errorf("no supported DB config found for identifier: type=%s, host=%s; at least one instance had a YAML parse error: %w",
			dbID.Type, dbID.Host, lastParseErr)
	}
	return nil, nil, instanceConfigIdentity{}, fmt.Errorf("no supported DB config found for identifier: type=%s, host=%s",
		dbID.Type, dbID.Host)
}

// findMatchingInstance searches cfg's instances for one matching dbID with data_observability
// enabled, returning the first match. Instances whose YAML fails to parse are skipped (logged and
// recorded as lastErr) rather than aborting the search — a later instance may still match.
func (c *component) findMatchingInstance(cfg *integration.Config, dbID *DBIdentifier) (map[string]any, instanceConfigIdentity, error) {
	if cfg.Name != "postgres" && cfg.Name != "sap_hana" {
		c.log.Warnf("DO query action: config %s is not a known DO-supported integration", cfg.Name)
	}

	var lastErr error
	for _, instanceData := range cfg.Instances {
		var instance map[string]any
		if err := yaml.Unmarshal(instanceData, &instance); err != nil {
			c.log.Warnf("Failed to unmarshal %s instance data for config %s, skipping: %v", cfg.Name, cfg.Name, err)
			lastErr = err
			continue
		}

		match := evaluateInstanceIdentifier(instance, *dbID, cfg.Name)
		c.log.Debugf(
			"Evaluated DO query action database identifier: integration=%s instance_host=%q target=%q strategy=%s rendered_database_identifier=%q renderable=%t matched=%t",
			cfg.Name,
			instanceHost(instance),
			dbID.Host,
			match.strategy,
			match.renderedIdentifier,
			match.renderable,
			match.matched,
		)
		if match.matched && instanceHasDOEnabled(instance) {
			return instance, identifyInstanceConfig(instanceData), nil
		}
	}
	return nil, instanceConfigIdentity{}, lastErr
}

// matchesIdentifier checks if a Postgres instance matches the given DB identifier. It uses the
// resolved database_instance value when configured and falls back to the literal host. Per-query
// dbname fields handle database routing.
func matchesIdentifier(instance map[string]any, dbID *DBIdentifier) bool {
	return instanceMatchesIdentifier(instance, *dbID, "postgres")
}

// instanceMatchesIdentifier reports whether an integration instance targets the given identifier.
// sap_hana uses "server" as the host key; postgres uses "host". The target host may be
// "host:port" (as sent by sap_hana backends) or bare "host", so we match against both the
// bare host and the "host:port" form built from the instance. Postgres also derives its
// database_instance from a configurable template. Render the same connection and tag values as
// the Python check and compare the resulting value.
//
// This is the single source of truth for selecting the instance to schedule.
func instanceMatchesIdentifier(instance map[string]any, identifier DBIdentifier, integrationName string) bool {
	return evaluateInstanceIdentifier(instance, identifier, integrationName).matched
}

type identifierMatchEvaluation struct {
	matched            bool
	strategy           string
	renderedIdentifier string
	renderable         bool
}

func evaluateInstanceIdentifier(instance map[string]any, identifier DBIdentifier, integrationName string) identifierMatchEvaluation {
	host := instanceHost(instance)
	if host == identifier.Host {
		return identifierMatchEvaluation{matched: true, strategy: "host"}
	}
	// Try matching "host:port" form — sap_hana backends include the port in the identifier.
	if port, ok := instancePort(instance); ok {
		if fmt.Sprintf("%s:%d", host, port) == identifier.Host {
			return identifierMatchEvaluation{matched: true, strategy: "host_port"}
		}
	}

	defaultTemplate := ""
	defaultPort := 0
	if integrationName == "postgres" {
		defaultTemplate = "$resolved_hostname"
		defaultPort = defaultPostgresPort
	}
	databaseIdentifier, ok := renderDatabaseIdentifier(instance, identifier.AgentHostname, defaultTemplate, defaultPort)
	return identifierMatchEvaluation{
		matched:            ok && databaseIdentifier == identifier.Host,
		strategy:           "database_identifier",
		renderedIdentifier: databaseIdentifier,
		renderable:         ok,
	}
}

// renderDatabaseIdentifier renders a database_identifier template using the same inputs as the
// Postgres check. Unknown variables stay in the result, matching Python's safe_substitute.
func renderDatabaseIdentifier(instance map[string]any, agentHostname, defaultTemplate string, defaultPort int) (string, bool) {
	return renderDatabaseIdentifierWithLookup(instance, agentHostname, defaultTemplate, defaultPort, net.LookupHost)
}

func renderDatabaseIdentifierWithLookup(
	instance map[string]any,
	agentHostname, defaultTemplate string,
	defaultPort int,
	lookupHost func(string) ([]string, error),
) (string, bool) {
	template := defaultTemplate
	if configuredIdentifier, exists := instance["database_identifier"]; exists {
		databaseIdentifier, ok := configuredIdentifier.(map[string]any)
		if !ok {
			return "", false
		}
		if configuredTemplate, exists := databaseIdentifier["template"]; exists {
			var ok bool
			template, ok = configuredTemplate.(string)
			if !ok {
				return "", false
			}
		}
	}
	if template == "" {
		return "", false
	}

	values := templateTagValues(instance)
	host := instanceHost(instance)
	values["host"] = host
	if port, ok := instancePort(instance); ok {
		values["port"] = strconv.Itoa(port)
	} else if _, configured := instance["port"]; !configured && defaultPort != 0 {
		values["port"] = strconv.Itoa(defaultPort)
	}

	var resolvedHostname string
	if reportedHostname, ok := instance["reported_hostname"].(string); ok && reportedHostname != "" {
		resolvedHostname = reportedHostname
	} else {
		resolvedHostname = resolveDatabaseHostname(host, agentHostname, lookupHost)
	}
	values["resolved_hostname"] = resolvedHostname

	return safeSubstitute(template, values), true
}

// resolveDatabaseHostname mirrors the Postgres check's resolve_db_host behavior used to build
// $resolved_hostname. Besides local and socket hosts, the check reports the Agent hostname when
// the database host and Agent hostname resolve to the same IPv4 address.
func resolveDatabaseHostname(host, agentHostname string, lookupHost func(string) ([]string, error)) string {
	if strings.HasSuffix(host, ".local") {
		return host
	}
	if isLocalDBHost(host) {
		if agentHostname != "" {
			return agentHostname
		}
		return host
	}
	if agentHostname == "" {
		return host
	}

	hostAddresses, err := lookupHost(host)
	if err != nil {
		return host
	}
	agentAddresses, err := lookupHost(agentHostname)
	if err != nil {
		return host
	}
	hostIPv4, hostOK := firstIPv4Address(hostAddresses)
	agentIPv4, agentOK := firstIPv4Address(agentAddresses)
	if hostOK && agentOK && hostIPv4 == agentIPv4 {
		return agentHostname
	}
	return host
}

func firstIPv4Address(addresses []string) (string, bool) {
	for _, address := range addresses {
		if ip := net.ParseIP(address); ip != nil {
			if ipv4 := ip.To4(); ipv4 != nil {
				return ipv4.String(), true
			}
		}
	}
	return "", false
}

// templateTagValues exposes each key:value instance tag as a template variable. Duplicate keys
// are sorted and joined with commas, matching the Postgres check.
func templateTagValues(instance map[string]any) map[string]string {
	var tags []string
	switch configuredTags := instance["tags"].(type) {
	case []any:
		for _, configuredTag := range configuredTags {
			if tag, ok := configuredTag.(string); ok {
				tags = append(tags, tag)
			}
		}
	case []string:
		tags = append(tags, configuredTags...)
	}
	sort.Strings(tags)

	values := make(map[string]string)
	for _, tag := range tags {
		key, value, found := strings.Cut(tag, ":")
		if !found {
			continue
		}
		if previous, exists := values[key]; exists {
			values[key] = previous + "," + value
		} else {
			values[key] = value
		}
	}
	return values
}

// isLocalDBHost matches the local-address cases handled by the Postgres check's host resolver.
func isLocalDBHost(host string) bool {
	if host == "" || host == "localhost" || strings.HasPrefix(host, "/") {
		return true
	}
	if zoneIndex := strings.LastIndex(host, "%"); zoneIndex >= 0 {
		host = host[:zoneIndex]
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	return ip != nil && ip.IsLoopback()
}

// safeSubstitute implements Python string.Template's $name, ${name}, and $$ forms. Unknown
// variables remain unchanged, matching safe_substitute.
func safeSubstitute(template string, values map[string]string) string {
	return databaseIdentifierVariablePattern.ReplaceAllStringFunc(template, func(placeholder string) string {
		if placeholder == "$$" {
			return "$"
		}
		name := strings.TrimSuffix(strings.TrimPrefix(placeholder, "${"), "}")
		if name == placeholder {
			name = strings.TrimPrefix(placeholder, "$")
		}
		if value, ok := values[name]; ok {
			return value
		}
		return placeholder
	})
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
	case string:
		port, err := strconv.Atoi(v)
		return port, err == nil
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
