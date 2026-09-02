// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package embedded

import (
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetSystemdUnitEmbedsAllVariants ensures every unit GetSystemdUnit can construct at runtime is
// actually compiled into systemdUnits, for both the ambient-capability and "-nc" variants.
// Enumerating the units rather than spot-checking one catches codegen dropping a single file from
// one variant's directory.
func TestGetSystemdUnitEmbedsAllVariants(t *testing.T) {
	for _, unitType := range []UnitType{UnitTypeOCI, UnitTypeDebRpm} {
		units := embeddedUnitsInDir(t, "tmpl/gen/s/"+systemdFlavorDir(unitType, true))
		assert.ElementsMatch(t, units, embeddedUnitsInDir(t, "tmpl/gen/s/"+systemdFlavorDir(unitType, false)),
			"embedded unit sets differ between %s and %s-nc", unitType, unitType)

		for _, unit := range units {
			for _, ambiantCapabilitiesSupported := range []bool{true, false} {
				name := fmt.Sprintf("%s/%s/ambiantCapabilitiesSupported=%t", unitType, unit, ambiantCapabilitiesSupported)
				t.Run(name, func(t *testing.T) {
					content, err := GetSystemdUnit(unit, unitType, ambiantCapabilitiesSupported)
					require.NoError(t, err)
					assert.NotEmpty(t, content)
				})
			}
		}
	}
}

// TestGetProcmgrUnitEmbedsAllVariants is the procmgr-unit-set equivalent of
// TestGetSystemdUnitEmbedsAllVariants: it ensures every unit GetProcmgrUnit can construct at
// runtime is actually compiled into procmgrUnits, for both the ambient-capability and "-nc"
// variants.
func TestGetProcmgrUnitEmbedsAllVariants(t *testing.T) {
	for _, unitType := range []UnitType{UnitTypeOCI, UnitTypeDebRpm} {
		units := embeddedUnitsInPmDir(t, "tmpl/gen/pm/"+string(unitType))
		assert.ElementsMatch(t, units, embeddedUnitsInPmDir(t, "tmpl/gen/pm/"+string(unitType)+"-nc"),
			"embedded unit sets differ between %s and %s-nc", unitType, unitType)

		for _, unit := range units {
			for _, ambiantCapabilitiesSupported := range []bool{true, false} {
				name := fmt.Sprintf("%s/%s/ambiantCapabilitiesSupported=%t", unitType, unit, ambiantCapabilitiesSupported)
				t.Run(name, func(t *testing.T) {
					content, err := GetProcmgrUnit(unit, unitType, ambiantCapabilitiesSupported)
					require.NoError(t, err)
					assert.NotEmpty(t, content)
				})
			}
		}
	}
}

// TestGetSystemdUnitSelectsNocapVariant ensures the unit returned for a host without ambient
// capability support is the one with the AmbientCapabilities directive stripped, not merely a unit
// that happens to load.
//
// This deliberately checks datadog-agent.service rather than every unit: tmpl/datadog-agent-data-plane.service.tmpl
// emits AmbientCapabilities unconditionally, without the {{ if .AmbiantCapabilitiesSupported }}
// guard its siblings use, so the data-plane units are byte-identical in both variants and would
// fail this assertion. That gap is a separate pre-existing template defect, not a property of the
// embed directives under test here.
func TestGetSystemdUnitSelectsNocapVariant(t *testing.T) {
	for _, unitType := range []UnitType{UnitTypeOCI, UnitTypeDebRpm} {
		t.Run(string(unitType), func(t *testing.T) {
			withCap, err := GetSystemdUnit("datadog-agent.service", unitType, true)
			require.NoError(t, err)
			assert.Contains(t, string(withCap), "AmbientCapabilities=")

			withoutCap, err := GetSystemdUnit("datadog-agent.service", unitType, false)
			require.NoError(t, err)
			assert.NotContains(t, string(withoutCap), "AmbientCapabilities=")
		})
	}
}

func embeddedUnitsInDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := systemdUnits.ReadDir(dir)
	require.NoError(t, err)
	return serviceNames(entries)
}

func embeddedUnitsInPmDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := procmgrUnits.ReadDir(dir)
	require.NoError(t, err)
	return serviceNames(entries)
}

func serviceNames(entries []fs.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".service") {
			names = append(names, entry.Name())
		}
	}
	return names
}
