// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config implements a component to handle trace-agent configuration.
package config

import (
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	traceconfig "github.com/DataDog/datadog-agent/pkg/trace/config"
)

// team: agent-apm

// Component is the component type.
type Component interface {
	// Warnings returns config warnings collected during setup.
	Warnings() *model.Warnings

	// SetHandler returns a handler for runtime configuration changes.
	SetHandler() http.Handler

	// GetConfigHandler returns a handler to fetch the runtime configuration.
	GetConfigHandler() http.Handler

	// SetMaxMemCPU
	SetMaxMemCPU(isContainerized bool)

	// Object returns wrapped config
	Object() *traceconfig.AgentConfig

	// OnUpdateAPIKey registers a callback for API Key changes
	OnUpdateAPIKey(func(oldKey, newKey string))
}

// Params defines the parameters for the config component.
type Params struct {
	// FailIfAPIKeyMissing controls if the Agent should fail if the API key is missing from the config.
	FailIfAPIKeyMissing bool
}
