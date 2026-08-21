// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package coat

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigratableServicesCatalog(t *testing.T) {
	require.NotEmpty(t, migratableServices)

	seen := make(map[string]struct{}, len(migratableServices))
	for _, service := range migratableServices {
		_, dup := seen[service.ID]
		assert.False(t, dup, "duplicate service id %q", service.ID)
		seen[service.ID] = struct{}{}

		assert.NotEmpty(t, service.ProcmgrProcessName)
		assert.NotEmpty(t, service.ProcmgrConfigFile)
		assert.NotEmpty(t, service.InstallMarkerRels)
		assert.NotEmpty(t, service.LegacySystemdUnits)
	}
}

func TestServiceByID(t *testing.T) {
	service, ok := serviceByID("process")
	require.True(t, ok)
	assert.Equal(t, "datadog-agent-process", service.ProcmgrProcessName)
	assert.Equal(t, "datadog-process-agent", service.LegacyWindowsService)
}
