// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package collectorimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/utils"
	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/externalhost"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
	"github.com/DataDog/datadog-agent/pkg/jmxfetch"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	jmxStatus "github.com/DataDog/datadog-agent/pkg/status/jmx"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"gopkg.in/yaml.v3"
)

//
// The "agent_check" metadata payload contains information about all running checks and the additional hostnames they
// added to the Agent.
//

const (
	defaultInterval   = 10 * time.Minute
	firstPayloadDelay = 1 * time.Minute
)

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	hostMetadataUtils.CommonPayload
	Meta             hostMetadataUtils.Meta `json:"meta"`
	AgentChecks      []interface{}          `json:"agent_checks"`
	ExternalhostTags externalhost.Payload   `json:"external_host_tags"`
}

// MarshalJSON serialization a Payload to JSON
func (p *Payload) MarshalJSON() ([]byte, error) {
	// use an alias to avoid infinite recursion while serializing
	type PayloadAlias Payload

	return json.Marshal((*PayloadAlias)(p))
}

// SplitPayload breaks the payload into times number of pieces
func (p *Payload) SplitPayload(_ int) ([]marshaler.AbstractMarshaler, error) {
	return nil, fmt.Errorf("AgentChecks Payload splitting is not implemented")
}

// GetPayload builds a payload of all the agentchecks metadata
func (c *collectorImpl) GetPayload(ctx context.Context) *Payload {
	hostnameData, _ := c.hostname.Get(ctx)

	meta := hostMetadataUtils.GetMetaFromCache(ctx, c.config, c.hostname)
	meta.Hostname = hostnameData

	cp := hostMetadataUtils.GetCommonPayload(hostnameData, c.config)
	payload := &Payload{
		CommonPayload:    *cp,
		Meta:             *meta,
		ExternalhostTags: *externalhost.GetPayload(),
	}

	checkStats := expvars.GetCheckStats()
	for _, stats := range checkStats {
		for _, s := range stats {
			var integrationTags []string
			if check, found := c.get(s.CheckID); found {
				var err error
				integrationTags, err = collectTags(check.InstanceConfig())
				if err != nil {
					log.Infof("Error collecting tags from check %s: %v", check, err)
				}
			}
			var status []interface{}
			if s.LastError != "" {
				status = []interface{}{
					s.CheckName, s.CheckName, s.CheckID, "ERROR", s.LastError, "", integrationTags,
				}
			} else if len(s.LastWarnings) != 0 {
				status = []interface{}{
					s.CheckName, s.CheckName, s.CheckID, "WARNING", s.LastWarnings, "", integrationTags,
				}
			} else {
				status = []interface{}{
					s.CheckName, s.CheckName, s.CheckID, "OK", "", "", integrationTags,
				}
			}
			payload.AgentChecks = append(payload.AgentChecks, status)
		}
	}

	loaderErrors := collector.GetLoaderErrors()
	for check, errs := range loaderErrors {
		jsonErrs, err := json.Marshal(errs)
		if err != nil {
			log.Warnf("Error formatting loader error from check %s: %v", check, err)
		}
		status := []interface{}{
			check, check, "initialization", "ERROR", string(jsonErrs),
		}
		payload.AgentChecks = append(payload.AgentChecks, status)
	}

	configErrors := autodiscoveryimpl.GetConfigErrors()
	for check, e := range configErrors {
		status := []interface{}{
			check, check, "initialization", "ERROR", e,
		}
		payload.AgentChecks = append(payload.AgentChecks, status)
	}

	jmxStartupError := jmxStatus.GetStartupError()
	if jmxStartupError.LastError != "" {
		status := []interface{}{
			"jmx", "jmx", "initialization", "ERROR", jmxStartupError.LastError,
		}
		payload.AgentChecks = append(payload.AgentChecks, status)
	}

	stats := map[string]interface{}{}
	jmxStatus.PopulateStatus(stats)
	instanceConfByName := map[string]interface{}{}
	for _, config := range jmxfetch.GetScheduledConfigs() {
		for _, instance := range config.Instances {
			instanceconfig := map[interface{}]interface{}{}
			err := yaml.Unmarshal(instance, &instanceconfig)
			if err != nil {
				log.Errorf("invalid instance section: %s", err)
				continue
			}
			log.Debugf("JMXStatus InstanceConfig %v", instanceconfig)
			if tagsNode, ok := instanceconfig["tags"]; ok {
				instanceConfByName[instanceconfig["name"].(string)] = tagsNode
			}
		}
	}

	if _, ok := stats["JMXStatus"]; ok {
		if status, ok := stats["JMXStatus"].(jmxStatus.Status); ok {
			for checkName, checksRaw := range status.ChecksStatus.InitializedChecks {
				checks, ok := checksRaw.([]interface{})
				if !ok {
					continue
				}
				for _, checkRaw := range checks {
					var tags interface{}
					check, ok := checkRaw.(map[string]interface{})
					// The default check status is OK, so if there is no status, it means the check is OK
					if !ok {
						continue
					}
					checkStatus, ok := check["status"].(string)
					if !ok {
						checkStatus = "OK"
					}
					checkID, ok := check["instance_name"].(string)
					if !ok {
						checkID = checkName
					} else {
						tags = instanceConfByName[checkID]
						checkID = fmt.Sprintf("%s:%s", checkName, checkID)
					}
					checkError, ok := check["message"].(string)
					if !ok {
						checkError = ""
					}

					status := []interface{}{
						checkName, checkName, checkID, checkStatus, checkError, "", tags,
					}
					payload.AgentChecks = append(payload.AgentChecks, status)
				}
			}
		}
	}
	return payload
}

func (c *collectorImpl) collectMetadata(ctx context.Context) time.Duration {
	metricSerializer, isSet := c.metricSerializer.Get()
	if !isSet {
		return defaultInterval
	}

	// We want to wait 1 min before collecting and sending the first payload.
	if time.Since(c.createdAt) < firstPayloadDelay {
		return firstPayloadDelay - time.Since(c.createdAt)
	}

	payload := c.GetPayload(ctx)
	if err := metricSerializer.SendAgentchecksMetadata(payload); err != nil {
		c.log.Errorf("unable to submit agentchecks metadata payload, %s", err)
	}
	return defaultInterval
}

type kv struct{ k, v string }

func extractTags(node *yaml.Node) []kv {
	if node == nil {
		return nil
	}

	if node.Kind == yaml.ScalarNode {
		return []kv{{v: node.Value}}
	}

	if node.Kind != yaml.SequenceNode {
		return nil
	}

	out := make([]kv, 0, len(node.Content))
	for _, item := range node.Content {
		switch item.Kind {
		case yaml.ScalarNode:
			out = append(out, kv{v: item.Value})
		case yaml.MappingNode:
			// Typical item is a one-key map; support multiple just in case.
			for i := 0; i+1 < len(item.Content); i += 2 {
				k := item.Content[i]
				v := item.Content[i+1]
				out = append(out, kv{k: k.Value, v: v.Value})
			}
		}
	}
	return out
}

// collectTags is a fast-path extractor assuming `tags` is at the root of the YAML.
// It decodes only the root-level `tags` field and returns as soon as it's found.
func collectTags(config string) ([]string, error) {
	if config == "" {
		return nil, nil
	}

	dec := yaml.NewDecoder(strings.NewReader(config))
	for {
		var doc yaml.Node
		if err := dec.Decode(&doc); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if len(doc.Content) == 0 {
			continue
		}
		root := doc.Content[0]
		if root.Kind != yaml.MappingNode {
			continue
		}
		for i := 0; i+1 < len(root.Content); i += 2 {
			k := root.Content[i]
			v := root.Content[i+1]
			if k.Kind == yaml.ScalarNode && k.Value == "tags" {
				group := extractTags(v)
				if len(group) == 0 {
					return nil, nil
				}
				out := make([]string, 0, len(group))
				for _, kv := range group {
					out = append(out, fmt.Sprintf("%s%s", kv.k, kv.v))
				}
				return out, nil
			}
		}
	}
	return nil, nil
}
