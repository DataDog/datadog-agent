// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package mock implements a fake integrations component to be used in tests
package mock

import (
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
)

type mockIntegrations struct {
}

// Subscribe returns an integrationLog channel
func (l *mockIntegrations) Subscribe() chan integrations.IntegrationLog {
	return make(chan integrations.IntegrationLog)
}

// SendLog does nothing
func (l *mockIntegrations) SendLog(_, _ string) {

}

// Mock returns a mock for integrations component.
func Mock() integrations.Component {
	return &mockIntegrations{}
}
