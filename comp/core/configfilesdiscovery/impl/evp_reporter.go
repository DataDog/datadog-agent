// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package configfilesdiscoveryimpl

import (
	"fmt"
	"time"

	"github.com/DataDog/agent-payload/v5/agentdiscovery"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type eventPlatformCollectedConfigSender struct {
	forwarder eventplatform.Forwarder
	hostID    string
}

func newEventPlatformCollectedConfigSender(eventPlatform eventplatform.Component, hostID string) collectedConfigSender {
	if eventPlatform == nil {
		log.Warnf("config files discovery Event Platform forwarder is unavailable, collected configs will not be sent")
		return noopCollectedConfigSender{}
	}

	forwarder, ok := eventPlatform.Get()
	if !ok || forwarder == nil {
		log.Warnf("config files discovery Event Platform forwarder is unavailable, collected configs will not be sent")
		return noopCollectedConfigSender{}
	}

	return &eventPlatformCollectedConfigSender{forwarder: forwarder, hostID: hostID}
}

func (r *eventPlatformCollectedConfigSender) SendCollectedConfigs(configs []collectedConfig) error {
	payloads := make([]*agentdiscovery.AgentDiscoveryPayload, 0, len(configs))
	for _, config := range configs {
		payload := agentDiscoveryPayloadFromCollectedConfig(config, time.Now())
		if payload == nil {
			continue
		}
		payloads = append(payloads, payload)
	}
	if len(payloads) == 0 {
		return nil
	}

	payloadBytes, err := proto.Marshal(&agentdiscovery.AgentDiscoveryPayloadBatch{
		HostId:   r.hostID,
		Payloads: payloads,
	})
	if err != nil {
		return fmt.Errorf("marshal agent discovery payload: %w", err)
	}

	msg := message.NewMessage(payloadBytes, nil, "", time.Now().UnixNano())
	if err := r.forwarder.SendEventPlatformEvent(msg, eventplatform.EventTypeAgentDiscovery); err != nil {
		return fmt.Errorf("send agent discovery payload to event platform: %w", err)
	}
	return nil
}

func agentDiscoveryPayloadFromCollectedConfig(config collectedConfig, ingestionTimestamp time.Time) *agentdiscovery.AgentDiscoveryPayload {
	if len(config.ConfigFiles) == 0 && len(config.EnvVars) == 0 {
		return nil
	}

	payload := &agentdiscovery.AgentDiscoveryPayload{
		Integration:        config.Integration,
		Runtime:            string(config.Runtime),
		RuntimeId:          config.RuntimeID,
		IngestionTimestamp: timestamppb.New(ingestionTimestamp),
		ConfigFiles:        make([]*agentdiscovery.AgentDiscoveryConfigFile, 0, len(config.ConfigFiles)),
		EnvVars:            make([]*agentdiscovery.AgentDiscoveryEnvVar, 0, len(config.EnvVars)),
	}
	for _, file := range config.ConfigFiles {
		payload.ConfigFiles = append(payload.ConfigFiles, &agentdiscovery.AgentDiscoveryConfigFile{
			Path:          file.Path,
			Content:       file.Content,
			Truncated:     file.Truncated,
			PayloadFormat: file.PayloadFormat,
		})
	}
	for _, envVar := range config.EnvVars {
		payload.EnvVars = append(payload.EnvVars, &agentdiscovery.AgentDiscoveryEnvVar{
			Name:  envVar.Name,
			Value: envVar.Value,
		})
	}
	return payload
}
