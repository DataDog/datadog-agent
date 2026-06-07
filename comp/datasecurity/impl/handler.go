// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package datasecurityimpl

import (
	"encoding/json"
	"errors"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/sds"
	yaml "gopkg.in/yaml.v3"
)

const (
	// postgresIntegrationName is the integration whose instance we take over and
	// enrich with the scan query. Only postgres is supported for now.
	postgresIntegrationName = "postgres"

	// dataSecuritySection is the key of the instance section that the query is
	// merged into and that carries the per-instance opt-in (enabled: true). The
	// section may already hold other settings (e.g. feature flags), which are
	// preserved.
	dataSecuritySection = "data_security"

	// dataSecurityQueryKey is the key, within the data_security section, that
	// holds the scan query forwarded from RC.
	dataSecurityQueryKey = "query"
)

// rcPayload is the DEBUG RC payload this component understands:
//
//	{
//	  "tasks": [
//	    {
//	      "scanning_rules": [ { "id": "...", "regex": "...", ... }, ... ],
//	      "scan_data": { "postgres": { "query": "SELECT ..." } }
//	    }
//	  ]
//	}
//
// The scanning rules reconfigure the Agent's unique Sensitive Data Scanner and
// are NOT forwarded to the check config. The postgres query is forwarded to the
// matching postgres instance.
type rcPayload struct {
	Tasks []rcTask `json:"tasks"`
}

type rcTask struct {
	ScanningRules []rcScanningRule `json:"scanning_rules"`
	ScanData      rcScanData       `json:"scan_data"`
}

type rcScanningRule struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Regex    string   `json:"regex"`
	Priority string   `json:"priority"`
	Tags     []string `json:"tags"`
	Labels   []string `json:"labels"`
}

type rcScanData struct {
	Postgres *rcPostgresScanData `json:"postgres"`
}

type rcPostgresScanData struct {
	Query string `json:"query"`
}

// takeover records, for a single handled RC config, what we did to autodiscovery:
//   - originals are the file-provided postgres configs we unscheduled. They are
//     both the source we rebuild enriched configs from on subsequent updates and
//     the configs we restore when the RC config goes away.
//   - scheduled are the enriched postgres configs we currently have scheduled.
type takeover struct {
	originals []integration.Config
	scheduled []integration.Config
}

// onUpdate is invoked by the RC client with the full set of active configs for
// the DEBUG product. For each payload we reconfigure the unique scanner with the
// scanning rules and take over the matching postgres config(s) with the scan
// query. Configs that disappear from a snapshot are reverted: the enriched copy
// is unscheduled and the original file config restored.
func (c *component) onUpdate(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) {
	changes := integration.ConfigChanges{}
	seen := make(map[string]bool, len(updates))

	for path, rawConfig := range updates {
		var payload rcPayload
		if err := json.Unmarshal(rawConfig.Config, &payload); err != nil {
			c.log.Warnf("datasecurity: failed to unmarshal DEBUG config %s: %v", path, err)
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}

		seen[path] = true

		if err := c.applyConfig(path, payload, &changes); err != nil {
			c.log.Warnf("datasecurity: cannot apply data_security config from %s: %v", path, err)
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			// Revert any takeover we previously performed for this path so we
			// don't leave the host without a postgres check.
			c.revertPath(path, &changes)
			continue
		}

		applyStatus(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}

	// Reconcile: revert previously handled configs absent from this snapshot.
	c.activeConfigsMu.Lock()
	var toRevert []string
	for path := range c.activeConfigs {
		if !seen[path] {
			toRevert = append(toRevert, path)
		}
	}
	c.activeConfigsMu.Unlock()

	for _, path := range toRevert {
		c.log.Infof("datasecurity: DEBUG config %s absent from RC snapshot, reverting", path)
		c.revertPath(path, &changes)
	}

	c.sendChanges(changes)
}

// applyConfig reconfigures the unique scanner with the rules carried by the
// payload and forwards the postgres scan query to the matching postgres
// config(s) by taking them over. On the first takeover for a path it
// unschedules the original file config(s); on subsequent updates it rebuilds the
// enriched copy from the remembered originals and unschedules the previously
// enriched copy.
func (c *component) applyConfig(path string, payload rcPayload, changes *integration.ConfigChanges) error {
	// Aggregate scanning rules across all tasks and the (first) postgres query.
	var rules []sds.RuleDefinition
	var query string
	for _, t := range payload.Tasks {
		for _, r := range t.ScanningRules {
			rules = append(rules, sds.RuleDefinition{
				ID:       r.ID,
				Name:     r.Name,
				Regex:    r.Regex,
				Priority: r.Priority,
				Tags:     r.Tags,
				Labels:   r.Labels,
			})
		}
		if query == "" && t.ScanData.Postgres != nil {
			query = t.ScanData.Postgres.Query
		}
	}

	// Reconfigure the unique, process-wide scanner. The rules are intentionally
	// not forwarded to the check config: the integration scans through this
	// scanner via the Agent.
	if len(rules) > 0 {
		if err := sds.Reconfigure(rules); err != nil {
			return errors.New("failed to reconfigure the sensitive data scanner")
		}
		c.log.Infof("datasecurity: reconfigured the default scanner with %d rule(s) from %s", len(rules), path)
	}

	if query == "" {
		// No postgres query to forward; nothing to schedule.
		return nil
	}

	c.activeConfigsMu.Lock()
	prev, existed := c.activeConfigs[path]
	c.activeConfigsMu.Unlock()

	// On the first takeover we discover the original file configs from
	// autodiscovery; afterwards we reuse the ones we remembered, since the
	// originals are no longer tracked by autodiscovery once we unschedule them.
	originals := prev.originals
	if !existed {
		originals = c.findPostgresConfigs()
		if len(originals) == 0 {
			return errors.New("no postgres instance with data_security.enabled: true found")
		}
	}

	enriched, err := c.buildEnrichedConfigs(originals, query)
	if err != nil {
		return err
	}

	if existed {
		// Replace the previously enriched copy; originals stay unscheduled.
		changes.Unschedule = append(changes.Unschedule, prev.scheduled...)
	} else {
		// First takeover: unschedule the original file config(s).
		changes.Unschedule = append(changes.Unschedule, originals...)
	}
	changes.Schedule = append(changes.Schedule, enriched...)

	c.activeConfigsMu.Lock()
	c.activeConfigs[path] = takeover{originals: originals, scheduled: enriched}
	c.activeConfigsMu.Unlock()

	c.log.Infof("datasecurity: forwarded data_security query to %d postgres config(s) (DEBUG config %s)", len(enriched), path)
	return nil
}

// revertPath undoes a takeover: it unschedules the enriched copies and restores
// the original file config(s). It is a no-op if path was never taken over.
func (c *component) revertPath(path string, changes *integration.ConfigChanges) {
	c.activeConfigsMu.Lock()
	prev, existed := c.activeConfigs[path]
	if existed {
		delete(c.activeConfigs, path)
	}
	c.activeConfigsMu.Unlock()

	if !existed {
		return
	}
	changes.Unschedule = append(changes.Unschedule, prev.scheduled...)
	changes.Schedule = append(changes.Schedule, prev.originals...)
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

// instanceHasQuery reports whether a parsed instance already carries a query in
// its data_security section. Such an instance is one we enriched ourselves, so
// it must not be taken over again as a base config (otherwise we would treat our
// own output as the original to restore on revert).
func instanceHasQuery(instance map[string]any) bool {
	ds, ok := instance[dataSecuritySection].(map[string]any)
	if !ok {
		return false
	}
	_, has := ds[dataSecurityQueryKey]
	return has
}

// findPostgresConfigs returns the postgres configs currently known to
// autodiscovery that have at least one instance with data_security.enabled:
// true. Configs we already enriched (their matching instance carries a query)
// are skipped so we never take over our own output.
func (c *component) findPostgresConfigs() []integration.Config {
	var out []integration.Config
	for _, cfg := range c.ac.GetUnresolvedConfigs() {
		if cfg.Name != postgresIntegrationName {
			continue
		}
		for _, instanceData := range cfg.Instances {
			var instance map[string]any
			if err := yaml.Unmarshal(instanceData, &instance); err != nil {
				continue
			}
			if instanceDataSecurityEnabled(instance) && !instanceHasQuery(instance) {
				out = append(out, cfg)
				break
			}
		}
	}
	return out
}

// buildEnrichedConfigs returns, for each original postgres config, a copy whose
// data_security-enabled instances have the query merged into their
// data_security section (preserving any keys already present there).
// Non-matching instances are copied verbatim so the rest of the config keeps
// running.
func (c *component) buildEnrichedConfigs(originals []integration.Config, query string) ([]integration.Config, error) {
	out := make([]integration.Config, 0, len(originals))
	for _, orig := range originals {
		newInstances := make([]integration.Data, 0, len(orig.Instances))
		enriched := false
		for _, instanceData := range orig.Instances {
			var instance map[string]any
			if err := yaml.Unmarshal(instanceData, &instance); err != nil {
				return nil, errors.New("failed to parse postgres instance")
			}

			if !instanceDataSecurityEnabled(instance) || instanceHasQuery(instance) {
				newInstances = append(newInstances, instanceData)
				continue
			}

			// Merge the query into the instance's existing data_security section,
			// preserving any keys already configured there (e.g. feature flags).
			dataSecurity, _ := instance[dataSecuritySection].(map[string]any)
			if dataSecurity == nil {
				dataSecurity = map[string]any{}
			}
			dataSecurity[dataSecurityQueryKey] = query
			instance[dataSecuritySection] = dataSecurity

			instanceYAML, err := yaml.Marshal(instance)
			if err != nil {
				return nil, errors.New("failed to marshal postgres instance")
			}
			newInstances = append(newInstances, integration.Data(instanceYAML))
			enriched = true
		}

		if !enriched {
			continue
		}

		newCfg := orig
		newCfg.Instances = newInstances
		newCfg.Source = c.String()
		out = append(out, newCfg)
	}

	if len(out) == 0 {
		return nil, errors.New("no postgres instance with data_security.enabled: true matched")
	}
	return out, nil
}
