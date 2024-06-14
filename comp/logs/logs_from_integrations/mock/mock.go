// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package mock

import (
	"testing"
	
	logs_from_integrations "github.com/DataDog/datadog-agent/comp/logs/logs_from_integrations/def"
)

// Mock returns a mock for logs_from_integrations component.
func Mock(t *testing.T) logs_from_integrations.Component {
	// TODO: Implement the logs_from_integrations mock
	return nil
}
