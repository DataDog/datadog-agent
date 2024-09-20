// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package integrations adds a go interface for integrations to register and
// send logs.
//
// The integrations component is a basic interface for integrations to send logs
// from one place to another. Integrations and their configs can be registered
// using the RegisterIntegrations function and then use the SendLog function to
// send logs to consumers, who will use the SubscribeIntegration and Subscribe
// functions to receive integration configs and logs.
package integrations

import "github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"

// team: agent-metrics-logs

// Component is the component type.
type Component interface {
	// RegisterIntegration registers an integration with the component.
	RegisterIntegration(id string, config integration.Config)

	// SubscribeIntegration returns a channel for a subscriber to receive integration configurations.
	SubscribeIntegration() chan IntegrationConfig

	// Subscribe subscribes returns a channel for a subscriber to receive logs from integrations.
	Subscribe() chan IntegrationLog

	// SendLog allows integrations to send logs to any subscribers.
	SendLog(log, integrationID string)
}
