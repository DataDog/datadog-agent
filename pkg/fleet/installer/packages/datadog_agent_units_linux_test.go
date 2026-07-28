// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package packages

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/procmgr"
)

var unitFlavors = []struct {
	unitType embedded.SystemdUnitType
	ambCaps  bool
}{
	{embedded.SystemdUnitTypeOCI, true},
	{embedded.SystemdUnitTypeOCI, false},
	{embedded.SystemdUnitTypeDebRpm, true},
	{embedded.SystemdUnitTypeDebRpm, false},
}

func flavorName(unitType embedded.SystemdUnitType, ambCaps bool) string {
	if ambCaps {
		return string(unitType)
	}
	return string(unitType) + "-nocap"
}

// TestUnitArraysResolveInTheirTree is the guard that the service catalogs and the generated unit
// trees change together: both write paths hard-fail on a missing embed, so a name in an array with
// no matching generated file breaks postInstall at runtime rather than at build time.
func TestUnitArraysResolveInTheirTree(t *testing.T) {
	catalogs := map[string]*datadogAgentService{
		"agentService":     &agentService,
		"agentDDOTService": &agentDDOTService,
	}

	for name, catalog := range catalogs {
		for _, flavor := range unitFlavors {
			label := fmt.Sprintf("%s/%s", name, flavorName(flavor.unitType, flavor.ambCaps))

			for _, unit := range append(catalog.SystemdUnitsStable, catalog.SystemdUnitsExp...) {
				_, err := embedded.GetSystemdUnit(unit, flavor.unitType, flavor.ambCaps)
				assert.NoError(t, err, "%s: %s missing from the systemd tree", label, unit)
			}
			for _, unit := range append(catalog.ProcmgrUnitsStable, catalog.ProcmgrUnitsExp...) {
				_, err := embedded.GetProcmgrUnit(unit, flavor.unitType, flavor.ambCaps)
				assert.NoError(t, err, "%s: %s missing from the procmgr tree", label, unit)
			}
		}
	}
}

// TestManagerUnitSetsAreDisjoint pins the split that replaces the old ConditionPathExists=! gate:
// the two managers never write each other's supervision unit.
func TestManagerUnitSetsAreDisjoint(t *testing.T) {
	for _, unit := range append(agentService.SystemdUnitsStable, agentService.SystemdUnitsExp...) {
		assert.NotContains(t, unit, "-procmgr", "plain systemd must not ship dd-procmgrd")
	}
	for _, unit := range append(agentService.ProcmgrUnitsStable, agentService.ProcmgrUnitsExp...) {
		assert.NotContains(t, unit, "-ddot", "procmgr supervises DDOT through processes.d, not a unit")
	}

	assert.Contains(t, agentService.SystemdUnitsStable, "datadog-agent-ddot.service")
	assert.Contains(t, agentService.SystemdUnitsExp, "datadog-agent-ddot-exp.service")
	assert.Contains(t, agentService.ProcmgrUnitsStable, "datadog-agent-procmgr.service")
	assert.Contains(t, agentService.ProcmgrUnitsExp, "datadog-agent-procmgr-exp.service")
}

// TestGeneratedWantsMatchesUnitArrays checks the generated main unit pulls up exactly the
// supervision path its tree declares. Without this, a leftover unit from the other manager could be
// started alongside the active one.
func TestGeneratedWantsMatchesUnitArrays(t *testing.T) {
	for _, flavor := range unitFlavors {
		label := flavorName(flavor.unitType, flavor.ambCaps)

		systemdStable := wantsLine(t, mustGetSystemdUnit(t, "datadog-agent.service", flavor.unitType, flavor.ambCaps))
		assert.Contains(t, systemdStable, "datadog-agent-ddot.service", label)
		assert.NotContains(t, systemdStable, "datadog-agent-procmgr.service", label)

		systemdExp := wantsLine(t, mustGetSystemdUnit(t, "datadog-agent-exp.service", flavor.unitType, flavor.ambCaps))
		assert.Contains(t, systemdExp, "datadog-agent-ddot-exp.service", label)
		assert.NotContains(t, systemdExp, "datadog-agent-procmgr-exp.service", label)

		procmgrStable := wantsLine(t, mustGetProcmgrUnit(t, "datadog-agent.service", flavor.unitType, flavor.ambCaps))
		assert.Contains(t, procmgrStable, "datadog-agent-procmgr.service", label)
		assert.NotContains(t, procmgrStable, "datadog-agent-ddot.service", label)

		procmgrExp := wantsLine(t, mustGetProcmgrUnit(t, "datadog-agent-exp.service", flavor.unitType, flavor.ambCaps))
		assert.Contains(t, procmgrExp, "datadog-agent-procmgr-exp.service", label)
		assert.NotContains(t, procmgrExp, "datadog-agent-ddot-exp.service", label)
	}
}

// TestDDOTSystemdUnitHasNoProcmgrGate checks the removal of the negative ConditionPathExists that
// used to make the systemd unit inspect procmgr's state directory.
func TestDDOTSystemdUnitHasNoProcmgrGate(t *testing.T) {
	for _, flavor := range unitFlavors {
		unit := string(mustGetSystemdUnit(t, "datadog-agent-ddot.service", flavor.unitType, flavor.ambCaps))
		assert.NotContains(t, unit, "processes.d", flavorName(flavor.unitType, flavor.ambCaps))
	}
}

// TestProcmgrProcessListsShareFileNames is the tripwire for the shared processes.d directory: during
// a config experiment the experiment link points at the stable version, so an -exp file name would
// leave the running daemon supervising both definitions.
func TestProcmgrProcessListsShareFileNames(t *testing.T) {
	assert.Equal(t, agentService.ProcmgrProcessesStable, agentService.ProcmgrProcessesExp)
	for _, process := range agentService.ProcmgrProcessesStable {
		assert.NotContains(t, process, "-exp")
	}
}

func TestProcmgrProcessesResolveInBothHalves(t *testing.T) {
	for _, unitType := range []embedded.SystemdUnitType{embedded.SystemdUnitTypeOCI, embedded.SystemdUnitTypeDebRpm} {
		for _, process := range agentService.ProcmgrProcessesStable {
			_, err := embedded.GetProcmgrConfig(process, unitType, false)
			assert.NoError(t, err, "%s: %s missing from processes.d", unitType, process)
		}
		for _, process := range agentService.ProcmgrProcessesExp {
			_, err := embedded.GetProcmgrConfig(process, unitType, true)
			assert.NoError(t, err, "%s: %s missing from processes-exp.d", unitType, process)
		}
	}
}

// TestProcmgrConfigDirMatchesUnit is the guard for the one invariant that spans a Go helper and a
// generated unit: procmgrInstallRoot decides where the installer writes processes.d entries, while
// DD_PM_CONFIG_DIR decides where the daemon reads them. If they drift, the installer writes
// definitions nobody loads, and no build or runtime error says so.
func TestProcmgrConfigDirMatchesUnit(t *testing.T) {
	tests := []struct {
		packageType PackageType
		unitType    embedded.SystemdUnitType
		experiment  bool
		unit        string
	}{
		{PackageTypeOCI, embedded.SystemdUnitTypeOCI, false, "datadog-agent-procmgr.service"},
		{PackageTypeOCI, embedded.SystemdUnitTypeOCI, true, "datadog-agent-procmgr-exp.service"},
		{PackageTypeDEB, embedded.SystemdUnitTypeDebRpm, false, "datadog-agent-procmgr.service"},
		{PackageTypeDEB, embedded.SystemdUnitTypeDebRpm, true, "datadog-agent-procmgr-exp.service"},
		{PackageTypeRPM, embedded.SystemdUnitTypeDebRpm, false, "datadog-agent-procmgr.service"},
		{PackageTypeRPM, embedded.SystemdUnitTypeDebRpm, true, "datadog-agent-procmgr-exp.service"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/%s", tt.packageType, tt.unit), func(t *testing.T) {
			root, err := procmgrInstallRoot(HookContext{PackageType: tt.packageType}, tt.experiment)
			require.NoError(t, err)

			unit := string(mustGetProcmgrUnit(t, tt.unit, tt.unitType, true))
			want := `Environment="DD_PM_CONFIG_DIR=` + filepath.Join(root, procmgr.ProcessesDirName) + `"`
			assert.Contains(t, unit, want)
		})
	}
}

func TestProcmgrInstallRootRejectsUnknownPackageType(t *testing.T) {
	_, err := procmgrInstallRoot(HookContext{PackageType: PackageType("bogus")}, false)
	assert.Error(t, err)
}

func mustGetSystemdUnit(t *testing.T, name string, unitType embedded.SystemdUnitType, ambCaps bool) []byte {
	t.Helper()
	content, err := embedded.GetSystemdUnit(name, unitType, ambCaps)
	require.NoError(t, err)
	return content
}

func mustGetProcmgrUnit(t *testing.T, name string, unitType embedded.SystemdUnitType, ambCaps bool) []byte {
	t.Helper()
	content, err := embedded.GetProcmgrUnit(name, unitType, ambCaps)
	require.NoError(t, err)
	return content
}

func wantsLine(t *testing.T, unit []byte) string {
	t.Helper()
	for _, line := range strings.Split(string(unit), "\n") {
		if strings.HasPrefix(line, "Wants=") {
			return line
		}
	}
	require.Fail(t, "unit has no Wants= line")
	return ""
}
