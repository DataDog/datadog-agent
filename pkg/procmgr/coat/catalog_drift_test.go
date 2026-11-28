// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package coat

import (
	"path"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/units"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/units/darwinunits"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/units/linuxunits"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/units/windowsunits"
)

// Every processes.d YAML the installer ships must be registered in migratableServices.
// Omitting one leaves flares and agent_service_* gauges silent for that service, which is how the
// PAR family shipped under dd-procmgrd without appearing in services[]. Catalog may list more
// entries than are shipped today (migration targets still on systemd), so this is ⊆, not equality.
//
// Lives here (root module) rather than in pkg/fleet/installer: that module cannot import the root
// package, and go mod tidy would otherwise resolve coat via the published datadog-agent module.
func TestShippedProcmgrConfigsAreInTheCOATCatalog(t *testing.T) {
	var shipped []string
	for _, u := range slices.Concat(linuxunits.All(), darwinunits.All(), windowsunits.All()) {
		if u.Manager == units.Procmgr && u.Template != "" && path.Ext(u.Name) == ".yaml" {
			shipped = append(shipped, u.Name)
		}
	}
	assert.NotEmpty(t, shipped)
	assert.Subset(t, ProcmgrConfigFiles(), shipped)
}

func TestLegacySupervisorsAreInTheCOATCatalog(t *testing.T) {
	for id := range legacySupervisors {
		_, ok := serviceByID(id)
		assert.True(t, ok, "legacy supervisor of unknown service %q", id)
	}
}
