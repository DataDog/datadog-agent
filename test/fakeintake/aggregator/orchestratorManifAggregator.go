// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"fmt"
	"time"

	agentmodel "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// OrchestratorManifestPayload is a payload type for the OrchestratorManif check
type OrchestratorManifestPayload struct {
	Type          agentmodel.MessageType
	CollectedTime time.Time

	Manifest                *agentmodel.Manifest
	ManifestParentCollector *agentmodel.CollectorManifest
}

func (p OrchestratorManifestPayload) name() string {
	return ""
}

// GetTags is not implemented for OrchestratorManifestPayload
func (p OrchestratorManifestPayload) GetTags() []string {
	return nil
}

// GetCollectedTime returns the time that the payload was received by the fake intake
func (p OrchestratorManifestPayload) GetCollectedTime() time.Time {
	return p.CollectedTime
}

// ParseOrchestratorManifestPayload parses an api.Payload into a list of OrchestratorManifestPayload
func ParseOrchestratorManifestPayload(payload api.Payload) ([]*OrchestratorManifestPayload, error) {
	msg, err := agentmodel.DecodeMessage(payload.Data)
	if err != nil {
		return nil, err
	}
	var collector *agentmodel.CollectorManifest
	switch body := msg.Body.(type) {
	case *agentmodel.CollectorManifest:
		collector = body
	case *agentmodel.CollectorManifestCRD:
		collector = body.Manifest
	case *agentmodel.CollectorManifestCR:
		collector = body.Manifest
	default:
		return nil, fmt.Errorf("unexpected type %s", msg.Header.Type)
	}
	var out []*OrchestratorManifestPayload
	for _, manifest := range collector.Manifests {
		out = append(out, &OrchestratorManifestPayload{
			Type:                    msg.Header.Type,
			CollectedTime:           payload.Timestamp,
			Manifest:                manifest,
			ManifestParentCollector: collector,
		})
	}
	return out, nil
}

// OrchestratorManifestAggregator is an Aggregator for OrchestratorManifestPayload
type OrchestratorManifestAggregator struct {
	Aggregator[*OrchestratorManifestPayload]
}

// NewOrchestratorManifestAggregator returns a new OrchestratorManifestAggregator
func NewOrchestratorManifestAggregator() OrchestratorManifestAggregator {
	return OrchestratorManifestAggregator{
		Aggregator: newAggregator(ParseOrchestratorManifestPayload),
	}
}
