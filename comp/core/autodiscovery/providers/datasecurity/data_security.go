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
	// exampleCheckName is the Rust check scheduled on a scan task. See
	// pkg/collector/sharedlibrary/rustchecks/checks/example.
	exampleCheckName = "example"

	postgresIntegrationName = "postgres"

	// TODO(data-security): DATA_SECURITY_DB_SCAN_TASKS does not exist yet. We subscribe to
	// the generic DEBUG product and gate on the `product` payload attribute matching this
	// value. Subscribe to the dedicated product directly and drop the gate once provisioned.
	dataSecurityDBScanTasksProduct = "DATA_SECURITY_DB_SCAN_TASKS"
)

// scanTaskPayload is the RC config received for a Data Security DB scan task.
type scanTaskPayload struct {
	Product      string       `json:"product"`
	DBIdentifier dbIdentifier `json:"db_identifier"`
}

// dbIdentifier identifies the database a scan task targets. Matching is by host.
type dbIdentifier struct {
	Type string `json:"type"`
	Host string `json:"host"`
}

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
// of the example Rust check (min_collection_interval: 0).
type controller struct {
	ac            autodiscovery.Component
	rcclient      rcclient.Component
	configChanges chan integration.ConfigChanges
	closeMutex    sync.RWMutex
	closed        bool
}

// NewController creates a new Data Security controller instance.
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
			// TODO(data-security): subscribe to the dedicated product once it exists; DEBUG is a stand-in.
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

// update resolves the target database's connection info and triggers the Data Security check.
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

		// TODO(data-security): gate on the `product` attribute to simulate the dedicated
		// product; remove once we subscribe to it directly.
		if payload.Product != dataSecurityDBScanTasksProduct {
			log.Infof("poc datasecurity provider: ignoring RC config %s: product %q is not %q", path, payload.Product, dataSecurityDBScanTasksProduct)
			continue
		}

		log.Infof("poc datasecurity provider: resolving postgres connection for scan task %s (db host=%q, type=%q)", path, payload.DBIdentifier.Host, payload.DBIdentifier.Type)
		connInfo, err := c.resolvePostgresConnection(payload.DBIdentifier)
		if err != nil {
			log.Warnf("poc datasecurity provider: failed to resolve postgres connection for Data Security scan task %s: %v", path, err)
			applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}

		// TODO(data-security): trigger the datasecurity rust check once it consumes DB
		// connection config. For now run `example` as a one-off, passing connInfo through
		// to exercise the whole flow.
		instance, err := yaml.Marshal(map[string]any{
			"min_collection_interval": 0,
			"remote_config_id":        rawConfig.Metadata.ID,
			"database_connection":     connInfo,
		})
		if err != nil {
			applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}

		// TODO(data-security): remove this debug log.
		log.Infof("Data Security would schedule check %q with instance:\n%s", exampleCheckName, string(instance))

		changes.Schedule = append(changes.Schedule, integration.Config{
			Name:      exampleCheckName,
			Source:    c.String(),
			Instances: []integration.Data{integration.Data(instance)},
		})
		log.Infof("poc datasecurity provider: scheduled check %q for scan task %s", exampleCheckName, path)
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

// resolvePostgresConnection finds the local postgres instance matching dbID (by host) and
// extracts its connection info.
func (c *controller) resolvePostgresConnection(dbID dbIdentifier) (map[string]any, error) {
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
			if dbID.Host != "" && host != dbID.Host {
				log.Infof("poc datasecurity provider: postgres instance host=%q does not match target host=%q, skipping", host, dbID.Host)
				continue
			}
			log.Infof("poc datasecurity provider: matched postgres instance host=%q for target host=%q", host, dbID.Host)
			return extractPostgresConnectionInfo(instance), nil
		}
	}
	log.Warnf("poc datasecurity provider: no postgres integration found with host=%q", dbID.Host)
	return nil, fmt.Errorf("postgres integration with host=%q not found", dbID.Host)
}

// extractPostgresConnectionInfo copies allow-listed connection fields out of a postgres instance.
func extractPostgresConnectionInfo(instance map[string]any) map[string]any {
	out := make(map[string]any)
	allowList := []string{
		"host",
		"port",
		"username",
		"password",
		"dbname",
		"ssl",
	}
	for _, k := range allowList {
		if v, ok := instance[k]; ok {
			out[k] = v
		}
	}
	return out
}
