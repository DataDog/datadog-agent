// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package configfilesdiscoveryimpl

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const configObjectTypeFile = "file"

type agentDiscoveryPayload struct {
	Integration string                       `json:"integration"`
	ServiceID   string                       `json:"service_id"`
	Runtime     RuntimeType                  `json:"runtime"`
	Configs     []agentDiscoveryConfigObject `json:"configs"`
}

type agentDiscoveryConfigObject struct {
	Type          string `json:"type"`
	Path          string `json:"path"`
	ContentBase64 string `json:"content_base64"`
	Truncated     bool   `json:"truncated"`
}

type eventPlatformConfigReporter struct {
	forwarder eventplatform.Forwarder
}

func newEventPlatformConfigReporter(eventPlatform eventplatform.Component) configCollectionReporter {
	if eventPlatform == nil {
		log.Warnf("config files discovery Event Platform forwarder is unavailable, collected configs will not be reported")
		return noopConfigCollectionReporter{}
	}

	forwarder, ok := eventPlatform.Get()
	if !ok || forwarder == nil {
		log.Warnf("config files discovery Event Platform forwarder is unavailable, collected configs will not be reported")
		return noopConfigCollectionReporter{}
	}

	return &eventPlatformConfigReporter{forwarder: forwarder}
}

func (r *eventPlatformConfigReporter) ReportConfigCollection(_ context.Context, report configCollectionReport) error {
	if len(report.Files) == 0 {
		return nil
	}

	payload := agentDiscoveryPayload{
		Integration: report.Integration,
		ServiceID:   report.ServiceID,
		Runtime:     report.Runtime,
		Configs:     make([]agentDiscoveryConfigObject, 0, len(report.Files)),
	}
	for _, file := range report.Files {
		payload.Configs = append(payload.Configs, agentDiscoveryConfigObject{
			Type:          configObjectTypeFile,
			Path:          file.Path,
			ContentBase64: base64.StdEncoding.EncodeToString(file.Content),
			Truncated:     file.Truncated,
		})
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal agent discovery payload: %w", err)
	}

	msg := message.NewMessage(payloadBytes, nil, "", time.Now().UnixNano())
	if err := r.forwarder.SendEventPlatformEventBlocking(msg, eventplatform.EventTypeAgentDiscovery); err != nil {
		return fmt.Errorf("send agent discovery payload to event platform: %w", err)
	}
	return nil
}
