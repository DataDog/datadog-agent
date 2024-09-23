// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package mock implements a fake integrations component to be used in tests
package mock

import (
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	integrationsimpl "github.com/DataDog/datadog-agent/comp/logs/integrations/impl"
)

type mockIntegrations struct {
	*integrationsimpl.Logsintegration
}

// Mock returns a mock for integrations component.
func Mock() integrations.Component {
	return &mockIntegrations{
		Logsintegration: integrationsimpl.NewLogsIntegration(),
	}
}
