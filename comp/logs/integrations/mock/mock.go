// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package mock implements a fake integrations component to be used in tests
package mock

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
)

type mockIntegrations struct {
	logChan chan integrations.IntegrationLog
}

func (l *mockIntegrations) Register(id string, config integration.Config) {
}

func (l *mockIntegrations) SubscribeIntegration() chan integrations.IntegrationConfig {
	return nil
}

// Subscribe returns an integrationLog channel
func (l *mockIntegrations) Subscribe() chan integrations.IntegrationLog {
	return l.logChan
}

// SendLog sends a log to the log channel
func (l *mockIntegrations) SendLog(log, integrationID string) {
	integrationLog := integrations.IntegrationLog{
		Log:           log,
		IntegrationID: integrationID,
	}

	l.logChan <- integrationLog
}

// Mock returns a mock for integrations component.
func Mock() integrations.Component {
	return &mockIntegrations{
		logChan: make(chan integrations.IntegrationLog),
	}
}
