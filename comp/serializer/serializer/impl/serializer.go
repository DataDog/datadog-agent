// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package serializerimpl implements the serializer component and all of its
// functional group components.
package serializerimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	defaultforwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	orchestratorforwarder "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/def"
	compression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	serializerdef "github.com/DataDog/datadog-agent/comp/serializer/serializer/def"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

// Requires defines the dependencies of the serializer component.
type Requires struct {
	Config                config.Component
	Log                   log.Component
	Forwarder             defaultforwarder.Component
	OrchestratorForwarder orchestratorforwarder.Component
	Compressor            compression.Component
	Hostname              hostnameinterface.Component
}

// Provides defines the components provided by the serializer. All fields are
// backed by the same instance.
type Provides struct {
	Comp         serializerdef.Component
	Metrics      serializer.MetricsSerializer
	Events       serializer.EventsSerializer
	Metadata     serializer.MetadataSerializer
	Orchestrator serializer.OrchestratorSerializer
}

// NewComponent creates the serializer.
func NewComponent(req Requires) (Provides, error) {
	routing, err := configutils.ReadMetricsEndpointRouting(req.Config)
	if err != nil {
		return Provides{}, err
	}
	// The hostname is only used for event payloads. Components that cannot
	// run without a hostname (e.g. the demultiplexer) enforce it themselves.
	hostname, err := req.Hostname.Get(context.TODO())
	if err != nil {
		req.Log.Warnf("Error getting hostname: %s", err)
		hostname = ""
	}

	// Some consumers type-assert the concrete *serializer.Serializer, so it is
	// provided as-is rather than wrapped.
	s := serializer.NewSerializerWithEndpointRouting(req.Forwarder, req.OrchestratorForwarder, req.Compressor, req.Config, req.Log, hostname, routing)
	return Provides{
		Comp:         s,
		Metrics:      s,
		Events:       s,
		Metadata:     s,
		Orchestrator: s,
	}, nil
}
