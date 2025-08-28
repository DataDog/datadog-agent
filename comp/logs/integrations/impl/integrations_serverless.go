// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build serverless

// Package integrationsimpl implements the integrations component interface
package integrationsimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
)

// Logsintegration is the integrations component implementation
type Logsintegration struct {
}

// NewLogsIntegration creates a new integrations instance
func NewLogsIntegration(_ log.Component, _ configComponent.Component) integrations.Component {
	return &Logsintegration{}
}

// RegisterIntegration registers an integration with the integrations component
func (li *Logsintegration) RegisterIntegration(_ string, _ integration.Config) {
}

// SendLog sends a log to any subscribers
func (li *Logsintegration) SendLog(_ string, _ string) {
}

// SetActionCallback sets the callback to be called when integration actions are performed.
func (li *Logsintegration) SetActionCallback(_ func() error) {
}

// Subscribe returns the channel that receives logs from integrations. Currently
// the integrations component only supports one subscriber, but can be extended
// later by making a new channel for any number of subscribers.
func (li *Logsintegration) Subscribe() chan integrations.IntegrationLog {
	return nil
}

// SubscribeIntegration returns the channel that receives integration configurations
func (li *Logsintegration) SubscribeIntegration() chan integrations.IntegrationConfig {
	return nil
}
