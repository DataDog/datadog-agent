// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package payloadmodifierimpl implements the trace payload modifier component
package payloadmodifierimpl

import (
	"go.uber.org/fx"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	payloadmodifier "github.com/DataDog/datadog-agent/comp/trace/payload-modifier/def"
	serverlessenv "github.com/DataDog/datadog-agent/pkg/serverless/env"
	serverlesstags "github.com/DataDog/datadog-agent/pkg/serverless/tags"
	serverlesstrace "github.com/DataDog/datadog-agent/pkg/serverless/trace"
	pkgagent "github.com/DataDog/datadog-agent/pkg/trace/agent"
)

// Dependencies holds the dependencies for the payload modifier component
type Dependencies struct {
	fx.In

	Config coreconfig.Component
}

type component struct {
	modifier pkgagent.TracerPayloadModifier
}

// NewComponent creates a new payload modifier component
func NewComponent(deps Dependencies) payloadmodifier.Component {
	var modifier pkgagent.TracerPayloadModifier

	// Only enable TracerPayloadModifier when running in Azure App Services
	// extension. serverless-init also uses the serverless
	// TracerPayloadModifier, but it instantiates its tracer without using fx,
	// so we don't need to worry about that here.
	if serverlessenv.IsAzureAppServicesExtension() {
		functionTags := serverlesstags.GetFunctionTags(deps.Config)
		modifier = serverlesstrace.NewTracerPayloadModifier(functionTags)
	}

	return &component{
		modifier: modifier,
	}
}

// GetModifier returns the TracerPayloadModifier instance, or nil if not enabled
func (c *component) GetModifier() pkgagent.TracerPayloadModifier {
	return c.modifier
}

