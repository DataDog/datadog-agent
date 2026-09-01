// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package payloadmodifierimpl implements the trace payload modifier component
package payloadmodifierimpl

import (
	"strings"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	payloadmodifier "github.com/DataDog/datadog-agent/comp/trace/payload-modifier/def"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	serverlessenv "github.com/DataDog/datadog-agent/pkg/serverless/env"
	serverlessmodifier "github.com/DataDog/datadog-agent/pkg/serverless/trace/modifier"
	pkgagent "github.com/DataDog/datadog-agent/pkg/trace/agent"
)

// Dependencies holds the dependencies for the payload modifier component
type Dependencies struct {
	Config coreconfig.Component
}

// Provides contains the payload modifier component
type Provides struct {
	Comp payloadmodifier.Component
}

type component struct {
	modifier pkgagent.TracerPayloadModifier
	// modifierV1 is the V1 (idx) equivalent of modifier, used by ProcessV1.
	modifierV1 pkgagent.TracerPayloadModifierV1
}

// NewComponent creates a new payload modifier component
func NewComponent(deps Dependencies) Provides {
	comp := &component{}

	// Only enable TracerPayloadModifier when running in Azure App Services
	// extension. serverless-init also uses the serverless
	// TracerPayloadModifier, but it instantiates its tracer without using fx,
	// so we don't need to worry about that here.
	if serverlessenv.IsAzureAppServicesExtension() {
		configuredTags := configUtils.GetConfiguredTags(deps.Config, false)
		functionTags := strings.Join(configuredTags, ",")
		modifier := serverlessmodifier.NewTracerPayloadModifier(functionTags)
		comp.modifier = modifier
		comp.modifierV1 = modifier
	}

	return Provides{
		Comp: comp,
	}
}

// Modify modifies the given TracerPayload, no-op if not enabled
func (c *component) Modify(payload *pb.TracerPayload) {
	if c.modifier != nil {
		c.modifier.Modify(payload)
	}
}

// ModifyV1 is the V1 (idx) equivalent of Modify, no-op if not enabled
func (c *component) ModifyV1(payload *idx.InternalTracerPayload) {
	if c.modifierV1 != nil {
		c.modifierV1.ModifyV1(payload)
	}
}
