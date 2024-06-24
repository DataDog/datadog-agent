// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package mock

import (
	"testing"

	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
)

type mock_integrations struct {
}

// Subscribe ...
func (l *mock_integrations) Subscribe() chan integrations.IntegrationLog {
	return make(chan integrations.IntegrationLog)
}

// SendLog
func (l *mock_integrations) SendLog(log, integrationID string) {

}

// Mock returns a mock for integrations component.
func Mock(t *testing.T) integrations.Component {
	// TODO: Implement the integrations mock
	return &mock_integrations{}
}
