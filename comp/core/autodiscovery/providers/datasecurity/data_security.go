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

// TODO(dsec-215): mutualize code principles with datastreams/kafka_actions.go.

const (
	// dataSecurityCheckName is the Rust shared-library check scheduled on a scan task.
	dataSecurityCheckName = "datasecurity"

	postgresIntegrationName = "postgres"
	postgresPlatform        = "postgres"
	// defaultPostgresPort is used when the matched instance omits the port.
	defaultPostgresPort = 5432
	// rcSubscriptionRetryInterval is how often we re-check for a postgres integration.
	rcSubscriptionRetryInterval = 10 * time.Second
)

// isConnectedToPostgres reports whether any postgres integration is configured.
func isConnectedToPostgres(ac autodiscovery.Component) bool {
	for _, config := range ac.GetAllConfigs() {
		if config.Name == postgresIntegrationName {
			return true
		}
	}
	return false
}

// controller schedules one-off datasecurity checks from Data Security DB scan task RC updates.
type controller struct {
	ac            autodiscovery.Component
	rcclient      rcclient.Component
	configChanges chan integration.ConfigChanges
	closeMutex    sync.RWMutex
	closed        bool

	// scheduledByPath holds the config scheduled per RC path: at most one task per path.
	scheduledByPath map[string]integration.Config
}

// NewController creates a Data Security controller. Only call it when both `data_security.enabled`
// and `shared_library_check.enabled` are set (see configUtils.IsDataSecurityEnabled).
func NewController(ac autodiscovery.Component, rcclient rcclient.Component) types.ConfigProvider {
	c := &controller{
		ac:       ac,
		rcclient: rcclient,
		// TODO(dsec-198): include backpressure to avoid blocking indefinitely
		configChanges:   make(chan integration.ConfigChanges, 10),
		scheduledByPath: make(map[string]integration.Config),
	}
	// Send an empty initial sync so Autodiscovery's config poller unblocks startup; real configs
	// are streamed later as scan tasks arrive over RC.
	c.configChanges <- integration.ConfigChanges{}
	// Subscribe immediately when a postgres integration is already configured; otherwise
	// poll until one appears.
	if !c.subscribeIfReady() {
		go c.manageSubscriptionToRC()
	}
	return c
}

// subscribeIfReady subscribes to the Data Security RC product once a postgres integration is
// configured. It reports whether the subscription happened.
func (c *controller) subscribeIfReady() bool {
	if !isConnectedToPostgres(c.ac) {
		return false
	}
	c.rcclient.Subscribe(data.ProductDataSecurityDBScanTasks, c.update)
	return true
}

// manageSubscriptionToRC polls until a postgres integration is configured, then subscribes to RC.
// TODO(dsec-198): change here to connect to RC in an event-driven fashion rather than polling
func (c *controller) manageSubscriptionToRC() {
	ticker := time.NewTicker(rcSubscriptionRetryInterval)
	defer ticker.Stop()
	for range ticker.C {
		c.closeMutex.RLock()
		if c.closed {
			c.closeMutex.RUnlock()
			return
		}
		c.closeMutex.RUnlock()
		if c.subscribeIfReady() {
			return
		}
	}
}

// String returns the provider name.
func (c *controller) String() string {
	return names.DataSecurity
}

// GetConfigErrors returns errors from the last update, shown in `agent status`.
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
	changes := integration.ConfigChanges{}
	for path, rawConfig := range updates {
		var payload scanTaskPayload
		if err := json.Unmarshal(rawConfig.Config, &payload); err != nil {
			// TODO(dsec-214): send sds-results to report task failure
			log.Errorf("failed to decode Data Security scan task from remote-config: %v", err)
			applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}

		// TODO(dsec-197): validate data security scan task payload if needed before building the check instance
		instance, err := c.buildCheckInstance(payload)
		if err != nil {
			// TODO(dsec-214): send sds-results to report task failure
			log.Warnf("failed to build datasecurity instance for scan task %s: %v", path, err)
			applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}

		newCfg := integration.Config{
			Name:      dataSecurityCheckName,
			Source:    c.String(),
			Instances: []integration.Data{integration.Data(instance)},
		}
		// Unschedule any config already scheduled for this path to avoid cumulating stale configs.
		if prev, ok := c.scheduledByPath[path]; ok {
			changes.Unschedule = append(changes.Unschedule, prev)
		}
		changes.Schedule = append(changes.Schedule, newCfg)
		c.scheduledByPath[path] = newCfg
		applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}

	if len(changes.Schedule) == 0 {
		return
	}

	c.closeMutex.RLock()
	defer c.closeMutex.RUnlock()
	if c.closed {
		return
	}
	c.configChanges <- changes
}

// buildCheckInstance resolves the local connection for every sub task and marshals the
// datasecurity check instance.
func (c *controller) buildCheckInstance(payload scanTaskPayload) ([]byte, error) {
	inst := checkInstance{
		// min_collection_interval: 0 runs the check once.
		MinCollectionInterval: 0,
		TaskID:                payload.TaskID,
		ScanningRules:         payload.ScanningRules,
		ScanData:              make([]checkSubTask, 0, len(payload.ScanData)),
	}

	for i := range payload.ScanData {
		st := payload.ScanData[i]
		if st.Entity.Platform != postgresPlatform {
			// TODO(dsec-214): send sds-results to report sub task failure
			return nil, fmt.Errorf("failed to build sub task %q: unsupported platform %q", st.SubTaskID, st.Entity.Platform)
		}

		conn, err := c.resolvePostgresConnection(st.Entity)
		if err != nil {
			// TODO(dsec-214): send sds-results to report sub task failure
			return nil, fmt.Errorf("failed to build sub task %q: %w", st.SubTaskID, err)
		}

		inst.ScanData = append(inst.ScanData, checkSubTask{
			subTask:    st,
			Connection: conn,
		})
	}

	// JSON is valid YAML (parsed by the check's serde_yaml) and emits the scanning_rule (json.RawMessage) as-is.
	return json.Marshal(inst)
}

// resolvePostgresConnection builds the scan connection from the local postgres instance
// matching the entity's host.
func (c *controller) resolvePostgresConnection(e entity) (connection, error) {
	for _, cfg := range c.ac.GetAllConfigs() {
		if cfg.Name != postgresIntegrationName {
			continue
		}
		for _, instanceData := range cfg.Instances {
			var instance map[string]any
			if err := yaml.Unmarshal(instanceData, &instance); err != nil {
				log.Warnf("skipping postgres instance: failed to unmarshal: %v", err)
				continue
			}
			if matchesHost(instance, e.DatabaseHostName) {
				return buildPostgresConnection(instance, e), nil
			}
		}
	}
	log.Warnf("no postgres integration found with host=%q", e.DatabaseHostName)
	return connection{}, fmt.Errorf("postgres integration with host=%q not found", e.DatabaseHostName)
}

// matchesHost reports whether a postgres instance targets the given host: an exact match
// or the "host:port" form some backends send.
func matchesHost(instance map[string]any, targetHost string) bool {
	host, _ := instance["host"].(string)
	if host == targetHost {
		return true
	}
	if port, ok := instancePort(instance); ok {
		return fmt.Sprintf("%s:%d", host, port) == targetHost
	}
	return false
}

// buildPostgresConnection copies credentials from the matched instance and targets the entity's database.
func buildPostgresConnection(instance map[string]any, e entity) connection {
	host, _ := instance["host"].(string)
	username, _ := instance["username"].(string)
	password, _ := instance["password"].(string)
	port, ok := instancePort(instance)
	if !ok {
		port = defaultPostgresPort
	}
	return connection{
		Host:     host,
		Port:     port,
		DBName:   e.Database,
		Username: username,
		Password: password,
	}
}

// instancePort returns the instance port, handling the numeric types YAML/JSON can produce
// (int, int64, float64).
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
