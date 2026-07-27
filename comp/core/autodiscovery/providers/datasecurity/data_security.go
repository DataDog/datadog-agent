// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package datasecurity schedules one-off Data Security checks triggered via Remote Configuration.
package datasecurity

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	rcclient "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/def"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	yaml "go.yaml.in/yaml/v2"
)

const (
	// dataSecurityCheckName is the Rust shared-library check scheduled on a scan task.
	// See pkg/collector/sharedlibrary/rustchecks/checks/datasecurity.
	dataSecurityCheckName = "datasecurity"

	postgresIntegrationName = "postgres"
	// postgresPlatform is the entity platform value backed by the postgres engine.
	postgresPlatform = "postgres"
)

// isConnectedToPostgres reports whether any postgres integration is configured.
func isConnectedToPostgres(ac autodiscovery.Component) bool {
	for _, config := range ac.GetUnresolvedConfigs() {
		if config.Name == postgresIntegrationName {
			return true
		}
	}
	return false
}

// controller listens to Data Security DB scan task RC updates and schedules a one-off run
// of the datasecurity Rust check (min_collection_interval: 0).
type controller struct {
	ac            autodiscovery.Component
	rcclient      rcclient.Component
	configChanges chan integration.ConfigChanges
	closeMutex    sync.RWMutex
	closed        bool
}

// NewController creates a new Data Security controller instance. The caller is
// responsible for only creating it when `data_security.enabled` is set (see the
// agent run command).
func NewController(ac autodiscovery.Component, rcclient rcclient.Component) types.ConfigProvider {
	c := &controller{
		ac:            ac,
		rcclient:      rcclient,
		configChanges: make(chan integration.ConfigChanges, 10),
	}
	c.configChanges <- integration.ConfigChanges{}
	go c.manageSubscriptionToRC()
	log.Infof("poc datasecurity provider: controller created, waiting for postgres integration before subscribing to RC")
	return c
}

// manageSubscriptionToRC waits until a postgres integration is configured before subscribing to RC.
func (c *controller) manageSubscriptionToRC() {
	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()
	for range ticker.C {
		c.closeMutex.RLock()
		if c.closed {
			c.closeMutex.RUnlock()
			return
		}
		c.closeMutex.RUnlock()
		if isConnectedToPostgres(c.ac) {
			// TODO(data-security): DATA_SECURITY_DB_SCAN_TASKS does not exist yet. We subscribe
			// to the generic DEBUG product for now; subscribe to the dedicated product once it
			// is provisioned.
			log.Infof("poc datasecurity provider: postgres integration detected, subscribing to RC product %q", data.ProductDebug)
			c.rcclient.Subscribe(data.ProductDebug, c.update)
			return
		}
		log.Infof("poc datasecurity provider: no postgres integration yet, will retry subscription")
	}
}

// String returns the provider name.
func (c *controller) String() string {
	return names.DataSecurity
}

// GetConfigErrors returns errors that occurred on the last update.
func (c *controller) GetConfigErrors() map[string]types.ErrorMsgSet {
	return map[string]types.ErrorMsgSet{}
}

// Stream sends configuration updates until the context is cancelled.
func (c *controller) Stream(ctx context.Context) <-chan integration.ConfigChanges {
	go func() {
		<-ctx.Done()
		c.closeMutex.Lock()
		defer c.closeMutex.Unlock()
		if c.closed {
			return
		}
		c.closed = true
		close(c.configChanges)
	}()
	return c.configChanges
}

// update translates each RC scan task into a datasecurity check instance and schedules it.
func (c *controller) update(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	log.Infof("poc datasecurity provider: received RC update with %d config(s)", len(updates))
	changes := integration.ConfigChanges{}
	for path, rawConfig := range updates {
		log.Infof("poc datasecurity provider: processing RC config %s", path)
		var payload scanTaskPayload
		if err := json.Unmarshal(rawConfig.Config, &payload); err != nil {
			log.Errorf("poc datasecurity provider: can't decode Data Security scan task from remote-config: %v", err)
			applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}

		// TODO(data-security): the DEBUG product is generic, so it carries configs that are
		// not scan tasks. Skip anything that does not look like a "data-security-db-scan-tasks"
		// payload. Drop this filter once we subscribe to the dedicated product.
		if payload.TaskID == "" || len(payload.ScanningRules) == 0 || len(payload.ScanData) == 0 {
			log.Infof("poc datasecurity provider: ignoring RC config %s: not a Data Security scan task", path)
			continue
		}

		instance, err := c.buildCheckInstance(payload)
		if err != nil {
			log.Warnf("poc datasecurity provider: failed to build datasecurity instance for scan task %s: %v", path, err)
			applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}

		// TODO(data-security): remove this debug log.
		log.Infof("Data Security would schedule check %q with instance:\n%s", dataSecurityCheckName, string(instance))

		changes.Schedule = append(changes.Schedule, integration.Config{
			Name:      dataSecurityCheckName,
			Source:    c.String(),
			Instances: []integration.Data{integration.Data(instance)},
		})
		log.Infof("poc datasecurity provider: scheduled check %q for scan task %s", dataSecurityCheckName, path)
		applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}

	if len(changes.Schedule) == 0 {
		log.Infof("poc datasecurity provider: no checks to schedule from this RC update")
		return
	}

	c.closeMutex.RLock()
	defer c.closeMutex.RUnlock()
	if c.closed {
		log.Infof("poc datasecurity provider: controller closed, dropping %d scheduled change(s)", len(changes.Schedule))
		return
	}
	log.Infof("poc datasecurity provider: pushing %d scheduled change(s) to autodiscovery", len(changes.Schedule))
	c.configChanges <- changes
}

// buildCheckInstance resolves the local connection for every sub task and marshals the
// datasecurity check instance. scanning_rules are forwarded to the check untouched.
func (c *controller) buildCheckInstance(payload scanTaskPayload) ([]byte, error) {
	inst := checkInstance{
		// min_collection_interval: 0 schedules the check as a one-off run.
		MinCollectionInterval: 0,
		TaskID:                payload.TaskID,
		ScanningRules:         payload.ScanningRules,
		ScanData:              make([]checkSubTask, 0, len(payload.ScanData)),
	}

	for i := range payload.ScanData {
		st := payload.ScanData[i]

		// TODO(data-security): only postgres is supported for now; add the other engines
		// (and route on entity.platform) as the check backends land.
		if st.Entity.Platform != postgresPlatform {
			return nil, fmt.Errorf("sub task %q: unsupported platform %q", st.SubTaskID, st.Entity.Platform)
		}

		conn, err := c.resolvePostgresConnection(st.Entity)
		if err != nil {
			return nil, fmt.Errorf("sub task %q: %w", st.SubTaskID, err)
		}

		// The check consumes the same sub task shape; only the resolved connection
		// is added on top.
		inst.ScanData = append(inst.ScanData, checkSubTask{
			subTask:    st,
			Connection: conn,
		})
	}

	// JSON is a valid YAML document, which both the agent (Configure) and the Rust
	// check (serde_yaml) parse. Marshalling with json keeps the snake_case field names
	// and passes scanning_rules through as raw dd-sds JSON.
	return json.Marshal(inst)
}

// resolvePostgresConnection finds the local postgres instance matching the entity
// (by host) and builds the connection used to scan the entity's database.
func (c *controller) resolvePostgresConnection(e entity) (connection, error) {
	for _, cfg := range c.ac.GetUnresolvedConfigs() {
		if cfg.Name != postgresIntegrationName {
			continue
		}
		log.Infof("poc datasecurity provider: inspecting postgres config with %d instance(s)", len(cfg.Instances))
		for _, instanceData := range cfg.Instances {
			var instance map[string]any
			if err := yaml.Unmarshal(instanceData, &instance); err != nil {
				log.Warnf("poc datasecurity provider: skipping postgres instance, failed to unmarshal: %v", err)
				continue
			}
			host, _ := instance["host"].(string)
			// An empty target host matches the first postgres instance found.
			if e.DatabaseHostName != "" && host != e.DatabaseHostName {
				log.Infof("poc datasecurity provider: postgres instance host=%q does not match target host=%q, skipping", host, e.DatabaseHostName)
				continue
			}
			log.Infof("poc datasecurity provider: matched postgres instance host=%q for target host=%q", host, e.DatabaseHostName)
			return buildPostgresConnection(instance, e), nil
		}
	}
	log.Warnf("poc datasecurity provider: no postgres integration found with host=%q", e.DatabaseHostName)
	return connection{}, fmt.Errorf("postgres integration with host=%q not found", e.DatabaseHostName)
}

// buildPostgresConnection copies the credentials from the matched postgres instance and
// targets the entity's database (the table to scan lives there).
func buildPostgresConnection(instance map[string]any, e entity) connection {
	host, _ := instance["host"].(string)
	username, _ := instance["username"].(string)
	password, _ := instance["password"].(string)
	return connection{
		Host:     host,
		Port:     toInt(instance["port"], 5432),
		DBName:   e.Database,
		Username: username,
		Password: password,
	}
}

// toInt coerces a value decoded from YAML (int, int64, uint16, float64) into an int,
// falling back to def when the value is missing or of an unexpected type.
func toInt(v any, def int) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case uint16:
		return int(n)
	case float64:
		return int(n)
	default:
		return def
	}
}
