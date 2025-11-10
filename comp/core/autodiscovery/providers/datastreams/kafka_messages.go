// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

// Package datastreams contains logic to configure actions for Kafka via remote configuration
package datastreams

import (
	"context"
	"encoding/json"
	"fmt"
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
	yaml "gopkg.in/yaml.v2"
)

const (
	kafkaConsumerIntegrationName = "kafka_consumer"
	kafkaActionsIntegrationName  = "kafka_actions"
)

// actionType enumerates the supported Kafka actions
type actionType string

const (
	actionRetrieve  actionType = "retrieve_messages"
	actionProduce   actionType = "produce_message"
	actionReplay    actionType = "replay_messages"
	actionManage    actionType = "manage_topic"
	actionEvolve    actionType = "evolve_schema"
	actionRebalance actionType = "rebalance_partitions"
)

// New structured remote configuration schema (no raw payloads), one action of each type per update
type kafkaActions struct {
	IntegrationDigest string                 `yaml:"integration_digest" json:"integration_digest"`
	Action            actionType             `yaml:"action" json:"action"` // enum: retrieve_messages, produce_message, replay_messages, manage_topic, evolve_schema, rebalance_partitions
	Retrieve          retrieveMessagesAction `yaml:"kafka_retrieve_messages" json:"kafka_retrieve_messages"`
	Produce           produceMessageAction   `yaml:"kafka_produce_message" json:"kafka_produce_message"`
	Replay            replayMessagesAction   `yaml:"kafka_replay_messages" json:"kafka_replay_messages"`
	Manage            manageTopicAction      `yaml:"kafka_manage_topic" json:"kafka_manage_topic"`
	Evolve            evolveSchemaAction     `yaml:"kafka_evolve_schema" json:"kafka_evolve_schema"`
	Rebalance         rebalanceAction        `yaml:"kafka_rebalance_partitions" json:"kafka_rebalance_partitions"`
}

type retrieveMessagesAction struct {
	Retrieve struct {
		Topic           string   `yaml:"topic" json:"topic"`
		Partition       int32    `yaml:"partition" json:"partition"`
		StartOffset     int64    `yaml:"start_offset" json:"start_offset"`
		MaxMessagesScan int32    `yaml:"max_messages_scan" json:"max_messages_scan"`
		MaxMessagesSend int32    `yaml:"max_messages_send" json:"max_messages_send"`
		TimeoutMs       int32    `yaml:"timeout_ms" json:"timeout_ms"`
		Filters         []filter `yaml:"filters" json:"filters"`
	} `yaml:"retrieve_messages" json:"retrieve_messages"`
}

type produceMessageAction struct {
	Produce struct {
		Topic     string            `yaml:"topic" json:"topic"`
		Value     string            `yaml:"value" json:"value"`
		Key       string            `yaml:"key" json:"key"`
		Partition int32             `yaml:"partition" json:"partition"`
		Headers   map[string]string `yaml:"headers" json:"headers"`
	} `yaml:"produce_message" json:"produce_message"`
}

type replayMessagesAction struct {
	Replay struct {
		SourceTopic       string   `yaml:"source_topic" json:"source_topic"`
		SourcePartition   int32    `yaml:"source_partition" json:"source_partition"`
		SourceStartOffset int64    `yaml:"source_start_offset" json:"source_start_offset"`
		SourceEndOffset   int64    `yaml:"source_end_offset" json:"source_end_offset"`
		DestTopic         string   `yaml:"dest_topic" json:"dest_topic"`
		DestPartition     int32    `yaml:"dest_partition" json:"dest_partition"`
		MaxMessages       int32    `yaml:"max_messages" json:"max_messages"`
		Filters           []filter `yaml:"filters" json:"filters"`
	} `yaml:"replay_messages" json:"replay_messages"`
}

type manageTopicAction struct {
	Manage struct {
		Operation         string            `yaml:"operation" json:"operation"`
		Topic             string            `yaml:"topic" json:"topic"`
		NumPartitions     int32             `yaml:"num_partitions" json:"num_partitions"`
		ReplicationFactor int16             `yaml:"replication_factor" json:"replication_factor"`
		Configs           map[string]string `yaml:"configs" json:"configs"`
	} `yaml:"manage_topic" json:"manage_topic"`
}

type evolveSchemaAction struct {
	Evolve struct {
		SchemaRegistryURL  string `yaml:"schema_registry_url" json:"schema_registry_url"`
		Subject            string `yaml:"subject" json:"subject"`
		SchemaType         string `yaml:"schema_type" json:"schema_type"`
		CompatibilityCheck bool   `yaml:"compatibility_check" json:"compatibility_check"`
		Schema             string `yaml:"schema" json:"schema"`
	} `yaml:"evolve_schema" json:"evolve_schema"`
}

type rebalanceAction struct {
	Rebalance struct {
		Topics   []string `yaml:"topics" json:"topics"`
		Brokers  []int32  `yaml:"brokers" json:"brokers"`
		Strategy string   `yaml:"strategy" json:"strategy"`
	} `yaml:"rebalance_partitions" json:"rebalance_partitions"`
}

type filter struct {
	Field    string `yaml:"field" json:"field"`
	Operator string `yaml:"operator" json:"operator"`
	Value    string `yaml:"value" json:"value"`
}

// controller listens to remote configuration updates for the Data Streams live messages feature
// and configures the kafka_consumer integration to fetch messages from Kafka.
type controller struct {
	ac            autodiscovery.Component
	rcclient      rcclient.Component
	configChanges chan integration.ConfigChanges
	closeMutex    sync.RWMutex
	closed        bool
}

// String returns the name of the provider.  All Config instances produced
// by this provider will have this value in their Provider field.
func (c *controller) String() string {
	return names.DataStreamsLiveMessages
}

// GetConfigErrors returns a map of errors that occurred on the last Collect
// call, indexed by a description of the resource that generated the error.
// The result is displayed in diagnostic tools such as `agent status`.
func (c *controller) GetConfigErrors() map[string]types.ErrorMsgSet {
	return map[string]types.ErrorMsgSet{}
}

// manageSubscriptionToRC subscribes to remote configuration updates if the agent is running the kafka_consumer integration.
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
		if isConnectedToKafka(c.ac) {
			log.Infof("[kafka_actions] Subscribing to remote config product DEBUG")
			c.rcclient.Subscribe(data.ProductDebug, c.update)
			return
		}
	}
}

func isConnectedToKafka(ac autodiscovery.Component) bool {
	for _, config := range ac.GetUnresolvedConfigs() {
		if config.Name == kafkaConsumerIntegrationName {
			return true
		}
	}
	return false
}

// NewController creates a new controller instance
func NewController(ac autodiscovery.Component, rcclient rcclient.Component) types.ConfigProvider {
	c := &controller{
		ac:            ac,
		rcclient:      rcclient,
		configChanges: make(chan integration.ConfigChanges, 10),
	}
	// Send an empty config change to ensure that config_poller starts correctly
	c.configChanges <- integration.ConfigChanges{}
	go c.manageSubscriptionToRC()
	return c
}

// Stream starts sending configuration updates for the kafka_consumer integration to the output channel.
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

// update parses updates from remote configuration, and schedules a one-off kafka_actions (Python) integration
// to act on Kafka according to the remote configuration. It no longer mutates existing kafka_consumer configs.
func (c *controller) update(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	log.Infof("[kafka_actions] Received remote config update with %d configs", len(updates))
	remoteConfigs := parseRemoteConfig(updates, applyStateCallback)
	if len(remoteConfigs) == 0 {
		log.Warnf("[kafka_actions] No valid remote configs parsed from update")
		return
	}
	log.Infof("[kafka_actions] Parsed %d valid kafka_actions configs", len(remoteConfigs))
	cfgs := c.ac.GetUnresolvedConfigs()
	changes := integration.ConfigChanges{}
	for i, rc := range remoteConfigs {
		log.Infof("[kafka_actions] Processing remote config %d: action=%s, integration_digest=%s", i+1, rc.Action, rc.IntegrationDigest)
		var base *integration.Config

		// Temporarily: copy auth from any kafka_consumer check (not just matching digest)
		for i := range cfgs {
			cfg := cfgs[i]
			if cfg.Name == kafkaConsumerIntegrationName {
				base = &cfg
				log.Infof("[kafka_actions] Found kafka_consumer integration: %s (digest=%s)", cfg.Name, cfg.Digest())
				break
			}
		}
		if base == nil {
			log.Warnf("[kafka_actions] Could not find any kafka_consumer integration")
		}

		// Extract broker/auth settings from the base kafka_consumer config (first instance)
		auth := extractKafkaAuth(base)
		log.Infof("[kafka_actions] Extracted %d auth/connection fields from base config", len(auth))
		// Build a single kafka_actions instance combining auth and exactly one action
		newCfg := integration.Config{
			Name:       kafkaActionsIntegrationName,
			Source:     c.String(),
			Instances:  []integration.Data{},
			InitConfig: nil,
			LogsConfig: nil,
		}
		if base != nil {
			newCfg.Provider = base.Provider
			newCfg.NodeName = base.NodeName
		}
		m := map[string]any{"run_once": true}
		// Select exactly one action based on enum
		switch rc.Action {
		case actionRetrieve:
			m["action"] = "retrieve_messages"
			m["retrieve_messages"] = rc.Retrieve.Retrieve
		case actionProduce:
			m["action"] = "produce_message"
			m["produce_message"] = rc.Produce.Produce
		case actionReplay:
			m["action"] = "replay_messages"
			m["replay_messages"] = rc.Replay.Replay
		case actionManage:
			m["action"] = "manage_topic"
			m["manage_topic"] = rc.Manage.Manage
		case actionEvolve:
			m["action"] = "evolve_schema"
			m["evolve_schema"] = rc.Evolve.Evolve
		case actionRebalance:
			m["action"] = "rebalance_partitions"
			m["rebalance_partitions"] = rc.Rebalance.Rebalance
		default:
			// No action configured; skip
			continue
		}
		for k, v := range auth {
			m[k] = v
		}
		payload, err := json.Marshal(m)
		if err != nil {
			log.Errorf("[kafka_actions] Failed to marshal instance payload: %v", err)
			continue
		}
		newCfg.Instances = []integration.Data{integration.Data(payload)}
		log.Infof("[kafka_actions] Scheduling kafka_actions check: action=%s, config=%s, payload_size=%d bytes", rc.Action, newCfg.String(), len(payload))
		log.Infof("[kafka_actions] Instance payload: %s", string(payload))
		changes.Schedule = append(changes.Schedule, newCfg)
	}
	if len(changes.Schedule) == 0 {
		log.Warnf("[kafka_actions] No configs to schedule after processing")
		return
	}
	log.Infof("[kafka_actions] Sending %d kafka_actions configs to scheduler", len(changes.Schedule))
	c.closeMutex.RLock()
	defer c.closeMutex.RUnlock()
	if c.closed {
		return
	}
	c.configChanges <- changes
}

func parseRemoteConfig(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) (configs []kafkaActions) {
	for path, rawConfig := range updates {
		var config kafkaActions
		err := json.Unmarshal(rawConfig.Config, &config)
		if err != nil {
			log.Errorf("Can't decode data streams live messages configuration provided by remote-config: %v", err)
			applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}
		configs = append(configs, config)
		applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}
	return configs
}

// extractKafkaAuth copies connection/authentication settings from a kafka_consumer config instance.
// It whitelists known keys and returns a flat map that can be merged into kafka_actions instance.
func extractKafkaAuth(base *integration.Config) map[string]any {
	out := make(map[string]any)
	if base == nil || len(base.Instances) == 0 {
		return out
	}
	raw := map[string]interface{}{}
	if err := yaml.Unmarshal(base.Instances[0], &raw); err != nil {
		return out
	}
	whitelist := []string{
		"kafka_connect_str", "bootstrap_servers",
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
	for _, k := range whitelist {
		if v, ok := raw[k]; ok {
			// Convert nested maps with interface{} keys to string keys
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
