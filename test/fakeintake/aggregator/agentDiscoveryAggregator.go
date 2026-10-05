// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package aggregator

import (
	"bytes"
	"time"

	"github.com/DataDog/agent-payload/v5/agentdiscovery"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"google.golang.org/protobuf/proto"
)

// AgentDiscoveryPayload represents an Agent Discovery payload sent through Event Platform.
type AgentDiscoveryPayload struct {
	collectedTime      time.Time
	Integration        string                     `json:"integration"`
	Runtime            string                     `json:"runtime"`
	HostID             string                     `json:"host_id"`
	RuntimeID          string                     `json:"runtime_id"`
	IngestionTimestamp time.Time                  `json:"ingestion_timestamp"`
	ConfigFiles        []AgentDiscoveryConfigFile `json:"config_files"`
	EnvVars            []AgentDiscoveryEnvVar     `json:"env_vars"`
}

// AgentDiscoveryConfigFile represents a discovered configuration file.
type AgentDiscoveryConfigFile struct {
	Path          string                                               `json:"path"`
	Content       []byte                                               `json:"content"`
	Truncated     bool                                                 `json:"truncated"`
	PayloadFormat agentdiscovery.AgentDiscoveryConfigFilePayloadFormat `json:"payload_format"`
}

// AgentDiscoveryEnvVar represents a discovered environment variable.
type AgentDiscoveryEnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func (p *AgentDiscoveryPayload) name() string {
	if p.Runtime == "" || p.RuntimeID == "" {
		return p.Integration
	}
	return p.Integration + ":" + p.Runtime + ":" + p.RuntimeID
}

// GetTags returns no tags for Agent Discovery payloads.
func (p *AgentDiscoveryPayload) GetTags() []string {
	return []string{}
}

// GetCollectedTime returns the time when the payload has been collected by the fakeintake server.
func (p *AgentDiscoveryPayload) GetCollectedTime() time.Time {
	return p.collectedTime
}

// ParseAgentDiscoveryPayload parses an api.Payload into Agent Discovery payloads.
func ParseAgentDiscoveryPayload(payload api.Payload) ([]*AgentDiscoveryPayload, error) {
	if len(payload.Data) == 0 {
		return []*AgentDiscoveryPayload{}, nil
	}

	inflated, err := inflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, err
	}
	if len(inflated) == 0 {
		return []*AgentDiscoveryPayload{}, nil
	}
	if bytes.Equal(bytes.TrimSpace(inflated), []byte("{}")) {
		return []*AgentDiscoveryPayload{}, nil
	}

	var batch agentdiscovery.AgentDiscoveryPayloadBatch
	if err := proto.Unmarshal(inflated, &batch); err != nil {
		return nil, err
	}

	payloads := make([]*AgentDiscoveryPayload, 0, len(batch.GetPayloads()))
	for _, protoPayload := range batch.GetPayloads() {
		parsedPayload := &AgentDiscoveryPayload{
			collectedTime: payload.Timestamp,
			Integration:   protoPayload.GetIntegration(),
			Runtime:       protoPayload.GetRuntime(),
			HostID:        batch.GetHostId(),
			RuntimeID:     protoPayload.GetRuntimeId(),
			ConfigFiles:   make([]AgentDiscoveryConfigFile, 0, len(protoPayload.GetConfigFiles())),
			EnvVars:       make([]AgentDiscoveryEnvVar, 0, len(protoPayload.GetEnvVars())),
		}
		if ingestionTimestamp := protoPayload.GetIngestionTimestamp(); ingestionTimestamp != nil {
			if err := ingestionTimestamp.CheckValid(); err != nil {
				return nil, err
			}
			parsedPayload.IngestionTimestamp = ingestionTimestamp.AsTime()
		}
		for _, configFile := range protoPayload.GetConfigFiles() {
			parsedPayload.ConfigFiles = append(parsedPayload.ConfigFiles, AgentDiscoveryConfigFile{
				Path:          configFile.GetPath(),
				Content:       configFile.GetContent(),
				Truncated:     configFile.GetTruncated(),
				PayloadFormat: configFile.GetPayloadFormat(),
			})
		}
		for _, envVar := range protoPayload.GetEnvVars() {
			parsedPayload.EnvVars = append(parsedPayload.EnvVars, AgentDiscoveryEnvVar{
				Name:  envVar.GetName(),
				Value: envVar.GetValue(),
			})
		}
		payloads = append(payloads, parsedPayload)
	}

	return payloads, nil
}

// AgentDiscoveryAggregator is an Aggregator for Agent Discovery payloads.
type AgentDiscoveryAggregator struct {
	Aggregator[*AgentDiscoveryPayload]
}

// NewAgentDiscoveryAggregator returns a new AgentDiscoveryAggregator.
func NewAgentDiscoveryAggregator() AgentDiscoveryAggregator {
	return AgentDiscoveryAggregator{
		Aggregator: newAggregator(ParseAgentDiscoveryPayload),
	}
}
