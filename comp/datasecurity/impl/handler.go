// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package datasecurityimpl

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	yaml "gopkg.in/yaml.v3"
)

const (
	// dataSecurityProductType is the product_type discriminator we act on.
	// DEBUG payloads carrying any other product_type are ignored.
	dataSecurityProductType = "data_security"

	// postgresIntegrationName is the integration whose instance we take over and
	// enrich with the data_security rules. Only postgres is supported for now.
	postgresIntegrationName = "postgres"
)

// debugPayload is the flat DEBUG RC payload this component understands:
//
//	{ "product_type": "data_security", "host": "...", "rules": [ ... ] }
//
// Host identifies the postgres instance to update. Rules are kept as raw JSON
// so they can be copied into the postgres instance as-is.
type debugPayload struct {
	ProductType string          `json:"product_type"`
	Host        string          `json:"host"`
	Rules       json.RawMessage `json:"rules"`
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
// the DEBUG product. The map is treated as a snapshot: only payloads with
// product_type == "data_security" are handled; every other payload is logged
// and ignored. For each handled payload we take over the matching postgres
// config(s): the original file config is unscheduled and an enriched copy (with
// the rules merged into its data_security section) is scheduled in its place,
// so a single, enriched postgres check runs. Configs that disappear from a
// snapshot are reverted: the enriched copy is unscheduled and the original file
// config restored. All resulting changes are delivered to autodiscovery at once.
func (c *component) onUpdate(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) {
	changes := integration.ConfigChanges{}
	seen := make(map[string]bool, len(updates))

	for path, rawConfig := range updates {
		var payload debugPayload
		if err := json.Unmarshal(rawConfig.Config, &payload); err != nil {
			c.log.Warnf("datasecurity: failed to unmarshal DEBUG config %s: %v", path, err)
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}

		if payload.ProductType != dataSecurityProductType {
			c.log.Infof(
				"datasecurity: ignoring DEBUG config %s with product_type=%q (only %q is handled)",
				path, payload.ProductType, dataSecurityProductType,
			)
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
			continue
		}

		seen[path] = true

		if err := c.applyConfig(path, payload, &changes); err != nil {
			c.log.Warnf("datasecurity: cannot apply data_security rules from %s: %v", path, err)
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

// applyConfig takes over the postgres config(s) matching the payload host and
// schedules an enriched copy. On the first takeover for a path it unschedules
// the original file config(s); on subsequent updates it rebuilds the enriched
// copy from the remembered originals and unschedules the previously enriched
// copy. The original file config(s) stay unscheduled until revertPath restores
// them.
func (c *component) applyConfig(path string, payload debugPayload, changes *integration.ConfigChanges) error {
	if payload.Host == "" {
		return fmt.Errorf("payload is missing the host of the postgres instance to update")
	}

	var rules any
	if len(payload.Rules) > 0 {
		if err := json.Unmarshal(payload.Rules, &rules); err != nil {
			return fmt.Errorf("failed to decode rules: %w", err)
		}
	}

	c.activeConfigsMu.Lock()
	prev, existed := c.activeConfigs[path]
	c.activeConfigsMu.Unlock()

	// On the first takeover we discover the original file configs from
	// autodiscovery; afterwards we reuse the ones we remembered, since the
	// originals are no longer tracked by autodiscovery once we unschedule them.
	originals := prev.originals
	if !existed {
		originals = c.findPostgresConfigs(payload.Host)
		if len(originals) == 0 {
			return fmt.Errorf("no postgres integration configured for host %q to attach data_security rules to", payload.Host)
		}
	}

	enriched, err := c.buildEnrichedConfigs(originals, payload.Host, rules)
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

	c.log.Infof("datasecurity: took over %d postgres config(s) for host %q with data_security rules (DEBUG config %s)", len(enriched), payload.Host, path)
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

// findPostgresConfigs returns the postgres configs currently known to
// autodiscovery that have at least one instance matching host. Configs this
// component scheduled itself are skipped so we never enrich our own output.
func (c *component) findPostgresConfigs(host string) []integration.Config {
	var out []integration.Config
	for _, cfg := range c.ac.GetUnresolvedConfigs() {
		if cfg.Name != postgresIntegrationName || cfg.Provider == names.DataSecurity {
			continue
		}
		for _, instanceData := range cfg.Instances {
			var instance map[string]any
			if err := yaml.Unmarshal(instanceData, &instance); err != nil {
				continue
			}
			if h, _ := instance["host"].(string); h == host {
				out = append(out, cfg)
				break
			}
		}
	}
	return out
}

// buildEnrichedConfigs returns, for each original postgres config, a copy whose
// instances matching host have the rules merged into their data_security
// section (preserving any keys already present there). Non-matching instances
// are copied verbatim so the rest of the config keeps running.
func (c *component) buildEnrichedConfigs(originals []integration.Config, host string, rules any) ([]integration.Config, error) {
	out := make([]integration.Config, 0, len(originals))
	for _, orig := range originals {
		newInstances := make([]integration.Data, 0, len(orig.Instances))
		enriched := false
		for _, instanceData := range orig.Instances {
			var instance map[string]any
			if err := yaml.Unmarshal(instanceData, &instance); err != nil {
				return nil, fmt.Errorf("failed to parse postgres instance: %w", err)
			}

			if h, _ := instance["host"].(string); h != host {
				newInstances = append(newInstances, instanceData)
				continue
			}

			// Enrich the instance's existing data_security section with the
			// rules as-is, preserving any keys already configured there.
			dataSecurity, _ := instance[dataSecurityProductType].(map[string]any)
			if dataSecurity == nil {
				dataSecurity = map[string]any{}
			}
			dataSecurity["rules"] = rules
			instance[dataSecurityProductType] = dataSecurity

			instanceYAML, err := yaml.Marshal(instance)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal postgres instance: %w", err)
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
		return nil, fmt.Errorf("no postgres instance matched host %q", host)
	}
	return out, nil
}
