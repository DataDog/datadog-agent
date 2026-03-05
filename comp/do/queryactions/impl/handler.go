// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package doqueryactionsimpl

import (
	"encoding/json"
	"fmt"
	"maps"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	yaml "gopkg.in/yaml.v2"
)

// dbCredentialAllowList defines the fields to extract from instance configs
var dbCredentialAllowList = []string{
	"host", "port", "username", "password", "dbname",
	"ssl", "ssl_mode", "ssl_cert", "ssl_key", "ssl_root_cert",
	"tls", "tls_verify", "tls_cert", "tls_key", "tls_ca_cert",
	"aws", "managed_authentication",
}

// isPostgresIntegration checks if the integration name is a supported postgres integration
func isPostgresIntegration(name string) bool {
	return name == "postgres"
}

// onDebugConfig handles RC DEBUG product updates with declarative config model.
// Each config represents the full set of active queries for a DB instance.
// An empty queries list signals that all queries for that config should be removed.
func (c *component) onDebugConfig(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) {
	for path, rawConfig := range updates {
		var payload DOQueryPayload
		if err := json.Unmarshal(rawConfig.Config, &payload); err != nil {
			c.log.Debugf("Failed to unmarshal DEBUG config %s: %v", path, err)
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}

		if !payload.DOQueryAction {
			c.log.Debugf("Skipping non-DO query action config: %s", path)
			continue
		}

		configID := payload.ConfigID
		c.log.Infof("Received DO query action config: %s (config_id: %s, queries: %d)", path, configID, len(payload.Queries))

		// Empty queries list → unschedule existing check for this config
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

		checkConfig := c.buildCheckConfig(&payload, baseCfg, instanceData, remoteConfigID)

		// Unschedule previous version of this config before scheduling new one
		c.unscheduleConfig(configID)

		c.closeMu.RLock()
		if !c.closed {
			c.configChanges <- integration.ConfigChanges{
				Schedule: []integration.Config{checkConfig},
			}
			c.activeConfigsMu.Lock()
			c.activeConfigs[configID] = checkConfig
			c.activeConfigsMu.Unlock()
			c.log.Infof("Scheduled DO query action check: %s (%d queries)", configID, len(payload.Queries))
		}
		c.closeMu.RUnlock()

		applyStatus(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}
}

// unscheduleConfig removes a previously scheduled check config by config ID
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
		c.configChanges <- integration.ConfigChanges{
			Unschedule: []integration.Config{prev},
		}
		c.log.Infof("Unscheduled DO query action check: %s", configID)
	}
	c.closeMu.RUnlock()
}

// findPostgresConfig finds a postgres config that matches the given identifier
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
				c.log.Debugf("Failed to unmarshal instance data: %v", err)
				continue
			}

			if matchesIdentifier(instance, dbID) {
				return &cfg, instanceData, nil
			}
		}
	}

	return nil, nil, fmt.Errorf("no postgres config found for identifier: type=%s, host=%s, port=%d, dbname=%s",
		dbID.Type, dbID.Host, dbID.Port, dbID.DBName)
}

// matchesDBName checks if an instance's dbname matches the RC identifier's dbname.
// If the RC has no dbname → match any instance. If the instance has no dbname → match.
// Otherwise, must match exactly.
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

// matchesIdentifier checks if an instance matches the given DB identifier
func matchesIdentifier(instance map[string]any, dbID *DBIdentifier) bool {
	switch dbID.Type {
	case "self-hosted":
		host, _ := instance["host"].(string)
		port := getPort(instance)
		return host == dbID.Host && port == dbID.Port && matchesDBName(instance, dbID)

	case "rds", "aurora":
		if dbID.DBInstanceIdentifier != "" {
			matched := false
			// Check direct dbinstanceidentifier field
			if id, ok := instance["dbinstanceidentifier"].(string); ok && id == dbID.DBInstanceIdentifier {
				matched = true
			}

			// Check tags for dbinstanceidentifier:<value>
			if !matched {
				if tags, ok := instance["tags"].([]any); ok {
					tagPrefix := "dbinstanceidentifier:" + dbID.DBInstanceIdentifier
					for _, tag := range tags {
						if tagStr, ok := tag.(string); ok && tagStr == tagPrefix {
							matched = true
							break
						}
					}
				}
			}

			// Check aws.instance_endpoint (YAML nested maps are map[any]any)
			if !matched {
				if awsConfig, ok := instance["aws"].(map[any]any); ok {
					if endpoint, ok := awsConfig["instance_endpoint"].(string); ok && endpoint == dbID.DBInstanceIdentifier {
						matched = true
					}
				}
			}

			if matched {
				return matchesDBName(instance, dbID)
			}
		}
		// Fall back to host/port matching
		host, _ := instance["host"].(string)
		port := getPort(instance)
		return host == dbID.Host && port == dbID.Port && matchesDBName(instance, dbID)
	}
	return false
}

// getPort extracts the port from an instance config
func getPort(instance map[string]any) int {
	switch p := instance["port"].(type) {
	case int:
		return p
	case float64:
		return int(p)
	case string:
		var port int
		fmt.Sscanf(p, "%d", &port)
		return port
	}
	return 0
}

// extractDBAuthFromInstanceData extracts credential fields from raw instance YAML using an allowlist
func extractDBAuthFromInstanceData(instanceData integration.Data) map[string]any {
	out := make(map[string]any)
	raw := map[string]interface{}{}
	if err := yaml.Unmarshal(instanceData, &raw); err != nil {
		return out
	}

	for _, k := range dbCredentialAllowList {
		v, ok := raw[k]
		if !ok {
			continue
		}
		// YAML nested maps are map[interface{}]interface{}; convert to map[string]interface{}
		if m, okm := v.(map[interface{}]interface{}); okm {
			strMap := make(map[string]interface{}, len(m))
			for kk, vv := range m {
				strMap[fmt.Sprint(kk)] = vv
			}
			out[k] = strMap
		} else {
			out[k] = v
		}
	}
	return out
}

// buildCheckConfig creates a check config for the do_query_actions check.
// The instance contains auth credentials and the full list of queries to schedule.
func (c *component) buildCheckConfig(payload *DOQueryPayload, baseCfg *integration.Config, instanceData integration.Data, remoteConfigID string) integration.Config {
	auth := extractDBAuthFromInstanceData(instanceData)

	// Build query list for the check instance
	queries := make([]map[string]any, 0, len(payload.Queries))
	for _, q := range payload.Queries {
		qm := map[string]any{
			"monitor_id":       q.MonitorID,
			"query":            q.Query,
			"interval_seconds": q.IntervalSeconds,
			"timeout_seconds":  q.TimeoutSeconds,
		}
		// Serialize entity metadata
		entBytes, err := json.Marshal(q.Entity)
		if err == nil {
			var entMap map[string]any
			if json.Unmarshal(entBytes, &entMap) == nil {
				qm["entity"] = entMap
			}
		}
		queries = append(queries, qm)
	}

	instanceFields := map[string]any{
		"remote_config_id": remoteConfigID,
		"db_type":          baseCfg.Name,
		"queries":          queries,
	}

	// Merge auth into instance fields (auth fields take precedence for connection)
	maps.Copy(instanceFields, auth)

	instanceYAML, err := yaml.Marshal(instanceFields)
	if err != nil {
		c.log.Errorf("Failed to marshal check instance: %v", err)
		instanceYAML = []byte{}
	}

	return integration.Config{
		Name:      "do_query_actions",
		Source:    c.String(),
		Provider:  baseCfg.Provider,
		NodeName:  baseCfg.NodeName,
		Instances: []integration.Data{integration.Data(instanceYAML)},
	}
}
