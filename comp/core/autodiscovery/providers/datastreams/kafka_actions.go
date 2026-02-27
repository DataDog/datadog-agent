// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

// Package datastreams contains logic to configure actions for Kafka via remote configuration
package datastreams

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	yaml "go.yaml.in/yaml/v2"
)

const (
	kafkaConsumerIntegrationName = "kafka_consumer"
	kafkaActionsIntegrationName  = "kafka_actions"
)

// isConnectedToKafka checks if any kafka_consumer integration is configured
func isConnectedToKafka(ac autodiscovery.Component) bool {
	for _, config := range ac.GetUnresolvedConfigs() {
		if config.Name == kafkaConsumerIntegrationName {
			return true
		}
	}
	return false
}

// legacyTagReplacer matches the same characters as the Python integration's
// TAG_REPLACEMENT regex: [,\+\*\-/()\[\]{}\s]
// This ensures the agent normalizes kafka_connect_str the same way the
// kafka_consumer Python check normalizes the bootstrap_servers tag value
// when enable_legacy_tags_normalization is true (the default).
var legacyTagReplacer = regexp.MustCompile(`[,+*\-/()\[\]{}\s]`)
var multipleUnderscoreCleanup = regexp.MustCompile(`_+`)
var dotUnderscoreCleanup = regexp.MustCompile(`\._`)

func normalizeBootstrapServers(servers string) string {
	if servers == "" {
		return ""
	}
	s := strings.ToLower(servers)
	s = legacyTagReplacer.ReplaceAllString(s, "_")
	s = multipleUnderscoreCleanup.ReplaceAllString(s, "_")
	s = dotUnderscoreCleanup.ReplaceAllString(s, ".")
	s = strings.Trim(s, "_")
	return s
}

type kafkaActionsConfig struct {
	Actions          json.RawMessage `json:"actions"`
	BootstrapServers string          `json:"bootstrap_servers"`
}

// actionsController listens to remote configuration updates for Kafka actions
// and schedules one-off kafka_actions checks.
type actionsController struct {
	ac            autodiscovery.Component
	rcclient      rcclient.Component
	configChanges chan integration.ConfigChanges
	closeMutex    sync.RWMutex
	closed        bool
}

// String returns the name of the provider. All Config instances produced
// by this provider will have this value in their Provider field.
func (c *actionsController) String() string {
	return names.DataStreamsLiveMessages
}

// GetConfigErrors returns a map of errors that occurred on the last Collect
// call, indexed by a description of the resource that generated the error.
// The result is displayed in diagnostic tools such as `agent status`.
func (c *actionsController) GetConfigErrors() map[string]types.ErrorMsgSet {
	return map[string]types.ErrorMsgSet{}
}

func (c *actionsController) manageSubscriptionToRC() {
	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()
	for range ticker.C {
		c.closeMutex.RLock()
		if c.closed {
			c.closeMutex.RUnlock()
			return
		}
		c.closeMutex.RUnlock()
		if isConnectedToKafka(c.ac) {
			c.rcclient.Subscribe(data.ProductDataStreamsKafkaActions, c.update)
			return
		}
	}
}

// NewActionsController creates a new Kafka actions controller instance
func NewActionsController(ac autodiscovery.Component, rcclient rcclient.Component) types.ConfigProvider {
	c := &actionsController{
		ac:            ac,
		rcclient:      rcclient,
		configChanges: make(chan integration.ConfigChanges, 10),
	}
	c.configChanges <- integration.ConfigChanges{}
	go c.manageSubscriptionToRC()
	return c
}

// Stream starts sending configuration updates for the kafka_actions integration to the output channel.
func (c *actionsController) Stream(ctx context.Context) <-chan integration.ConfigChanges {
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

func (c *actionsController) update(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	remoteConfigs := parseActionsConfig(updates, applyStateCallback)
	if len(remoteConfigs) == 0 {
		return
	}
	cfgs := c.ac.GetUnresolvedConfigs()
	changes := integration.ConfigChanges{}
	for _, parsed := range remoteConfigs {
		auth, base, err := extractKafkaAuthFromInstance(cfgs, parsed.bootstrapServers)
		if err != nil {
			log.Errorf("Failed to extract Kafka auth for config %s: %v", parsed.path, err)
			applyStateCallback(parsed.path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}

		newCfg := integration.Config{
			Name:       kafkaActionsIntegrationName,
			Source:     c.String(),
			Instances:  []integration.Data{},
			InitConfig: nil,
			LogsConfig: nil,
			Provider:   base.Provider,
			NodeName:   base.NodeName,
		}

		var actionsMap map[string]any
		decoder := json.NewDecoder(bytes.NewReader(parsed.actionsJSON))
		decoder.UseNumber()
		if err := decoder.Decode(&actionsMap); err != nil {
			log.Errorf("Failed to unmarshal actions JSON for config %s: %v", parsed.path, err)
			applyStateCallback(parsed.path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}

		// Copy auth fields first
		maps.Copy(actionsMap, auth)
		actionsMap["run_once"] = true
		actionsMap["remote_config_id"] = parsed.remoteConfigID

		payload, err := yaml.Marshal(actionsMap)
		if err != nil {
			log.Errorf("Failed to marshal instance config for %s: %v", parsed.path, err)
			applyStateCallback(parsed.path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}

		newCfg.Instances = []integration.Data{integration.Data(payload)}
		changes.Schedule = append(changes.Schedule, newCfg)
		applyStateCallback(parsed.path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
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

type parsedActionsConfig struct {
	path             string
	bootstrapServers string
	actionsJSON      json.RawMessage
	remoteConfigID   string
}

func parseActionsConfig(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) []parsedActionsConfig {
	var configs []parsedActionsConfig
	for path, rawConfig := range updates {
		var cfg kafkaActionsConfig
		err := json.Unmarshal(rawConfig.Config, &cfg)
		if err != nil {
			log.Errorf("Can't decode kafka actions configuration from remote-config: %v", err)
			applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}
		if len(cfg.Actions) == 0 {
			applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateError, Error: "missing actions"})
			continue
		}

		bootstrapServers := normalizeBootstrapServers(cfg.BootstrapServers)

		configs = append(configs, parsedActionsConfig{
			path:             path,
			bootstrapServers: bootstrapServers,
			actionsJSON:      cfg.Actions,
			remoteConfigID:   rawConfig.Metadata.ID,
		})
	}
	return configs
}

func extractKafkaAuthFromInstance(cfgs []integration.Config, bootstrapServers string) (map[string]any, *integration.Config, error) {
	out := make(map[string]any)

	for cfgIdx := range cfgs {
		cfg := cfgs[cfgIdx]
		if cfg.Name != kafkaConsumerIntegrationName {
			continue
		}

		// This is a special case, to be deleted if matching by bootstrap_servers works perfectly.
		// It is a fallback in case matching by bootstrap_servers fails in some cases.
		if bootstrapServers == "" {
			if len(cfg.Instances) > 0 {
				auth := extractAuthFromInstanceData(cfg.Instances[0])
				return auth, &cfg, nil
			}
			continue
		}

		for _, instanceData := range cfg.Instances {
			var instanceMap map[string]any
			if err := yaml.Unmarshal(instanceData, &instanceMap); err != nil {
				continue
			}

			var connectStrs []string
			switch v := instanceMap["kafka_connect_str"].(type) {
			case string:
				if v != "" {
					connectStrs = []string{v}
				}
			case []interface{}:
				for _, item := range v {
					if s, ok := item.(string); ok && s != "" {
						connectStrs = append(connectStrs, s)
					}
				}
			}

			for _, connectStr := range connectStrs {
				if normalizeBootstrapServers(connectStr) == bootstrapServers {
					auth := extractAuthFromInstanceData(instanceData)
					return auth, &cfg, nil
				}
			}
		}
	}

	if bootstrapServers == "" {
		return out, nil, errors.New("kafka_consumer integration not found on this node")
	}
	return out, nil, fmt.Errorf("kafka_consumer integration with bootstrap_servers=%s not found", bootstrapServers)
}

func extractAuthFromInstanceData(instanceData integration.Data) map[string]any {
	out := make(map[string]any)
	raw := map[string]interface{}{}
	if err := yaml.Unmarshal(instanceData, &raw); err != nil {
		return out
	}

	allowList := []string{
		"kafka_connect_str",
		"security_protocol",
		"sasl_mechanism",
		"sasl_plain_username",
		"sasl_plain_password",
		"sasl_kerberos_keytab",
		"sasl_kerberos_principal",
		"sasl_kerberos_service_name",
		"sasl_kerberos_domain_name",
		"tls_verify",
		"tls_ca_cert",
		"tls_cert",
		"tls_private_key",
		"tls_private_key_password",
		"tls_validate_hostname",
		"tls_ciphers",
		"tls_crlfile",
		"sasl_oauth_token_provider",
	}
	for _, k := range allowList {
		if v, ok := raw[k]; ok {
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
	}
	return out
}
