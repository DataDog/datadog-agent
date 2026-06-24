// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package integrationsimpl implements the integrations component interface
package integrationsimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	logagentconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
)

// Requires defines the dependencies for the integrations component
type Requires struct {
	compdef.In

	Config configComponent.Component
}

// Provides defines the output of the integrations component constructor
type Provides struct {
	compdef.Out

	Comp integrations.Component
}

// Logsintegration is the integrations component implementation
type Logsintegration struct {
	// disabled is true when the logs agent is not enabled; send methods become
	// no-ops so callers never block on channels that would never be drained.
	disabled        bool
	logChan         chan integrations.IntegrationLog
	integrationChan chan integrations.IntegrationConfig
}

// NewComponent creates a new integrations component.
func NewComponent(deps Requires) Provides {
	return Provides{
		Comp: &Logsintegration{
			disabled:        !logagentconfig.IsLogsEnabled(deps.Config),
			logChan:         make(chan integrations.IntegrationLog),
			integrationChan: make(chan integrations.IntegrationConfig),
		},
	}
}

// NewLogsIntegration creates a new integrations instance.
// Deprecated: use NewComponent instead.
func NewLogsIntegration() *Logsintegration {
	return &Logsintegration{
		logChan:         make(chan integrations.IntegrationLog),
		integrationChan: make(chan integrations.IntegrationConfig),
	}
}

// RegisterIntegration registers an integration with the integrations component
func (li *Logsintegration) RegisterIntegration(id string, config integration.Config) {
	if li.disabled || len(config.LogsConfig) == 0 {
		return
	}

	integrationConfig := integrations.IntegrationConfig{
		IntegrationID: id,
		Config:        config,
	}

	li.integrationChan <- integrationConfig
}

// SendLog sends a log to any subscribers
func (li *Logsintegration) SendLog(log, integrationID string) {
	if li.disabled {
		return
	}
	integrationLog := integrations.IntegrationLog{
		Log:           log,
		IntegrationID: integrationID,
	}

	li.logChan <- integrationLog
}

// Subscribe returns the channel that receives logs from integrations. Currently
// the integrations component only supports one subscriber, but can be extended
// later by making a new channel for any number of subscribers.
func (li *Logsintegration) Subscribe() chan integrations.IntegrationLog {
	return li.logChan
}

// SubscribeIntegration returns the channel that receives integration configurations
func (li *Logsintegration) SubscribeIntegration() chan integrations.IntegrationConfig {
	return li.integrationChan
}
