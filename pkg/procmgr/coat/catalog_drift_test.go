// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package coat

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
)

// Every processes.d YAML the installer ships must be registered in migratableServices.
// Omitting one leaves flares and agent_service_* gauges silent for that service, which is how the
// PAR family shipped under dd-procmgrd without appearing in services[]. Catalog may list more
// entries than are shipped today (migration targets still on systemd), so this is ⊆, not equality.
//
// Lives here (root module) rather than in pkg/fleet/installer: that module cannot import the root
// package, and go mod tidy would otherwise resolve coat via the published datadog-agent module.
func TestShippedProcmgrConfigsAreInTheCOATCatalog(t *testing.T) {
	shipped, err := embedded.ShippedProcmgrConfigFiles()
	require.NoError(t, err)
	require.NotEmpty(t, shipped, "installer embeds must ship at least one processes.d entry")

	registered := map[string]struct{}{}
	for _, name := range ProcmgrConfigFiles() {
		registered[name] = struct{}{}
	}

	var missing []string
	for _, name := range shipped {
		if _, ok := registered[name]; !ok {
			missing = append(missing, name)
		}
	}
	assert.Empty(t, missing,
		"processes.d configs shipped by the installer but absent from migratableServices: %v",
		missing)
}
