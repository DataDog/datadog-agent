// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package embedded

import (
	"fmt"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// embeddedUnits lists the .service files embedded under tmpl/gen/<dir>. It reads the embed.FS
// rather than the filesystem, so a missing //go:embed directive or embedsrcs entry shows up here.
// path.Join, not filepath.Join: embed.FS names are always slash-separated, so the backslashes
// filepath.Join emits on Windows would never resolve.
func embeddedUnits(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := systemdUnits.ReadDir(path.Join("tmpl/gen", dir))
	require.NoError(t, err)

	units := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".service") {
			units = append(units, entry.Name())
		}
	}
	require.NotEmpty(t, units, "no .service files embedded for %s", dir)
	return units
}

// TestGetSystemdUnitEmbedsAllVariants ensures every unit in every directory GetSystemdUnit can
// construct at runtime is actually compiled into systemdUnits. The "-nocap" variants are only
// selected on hosts whose kernel predates ambient capabilities, so a missing //go:embed directive
// for them is invisible to every test that reads the template tree from disk instead of from the
// embed.FS. Enumerating the units rather than spot-checking one also catches codegen dropping a
// single file from a "-nocap" directory, which the glob-based embed cannot detect on its own.
//
// The base and "-nocap" sets are compared against each other because the two are embedded from
// different sources under Bazel: BUILD.bazel lists the base units explicitly in service_srcs but
// globs the "-nocap" ones. Enumerating only one side would leave codegen additions missing from
// the other side invisible.
func TestGetSystemdUnitEmbedsAllVariants(t *testing.T) {
	for _, unitType := range []SystemdUnitType{SystemdUnitTypeOCI, SystemdUnitTypeDebRpm} {
		units := embeddedUnits(t, string(unitType))
		assert.ElementsMatch(t, units, embeddedUnits(t, string(unitType)+"-nocap"),
			"embedded unit sets differ between %s and %s-nocap", unitType, unitType)

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
	for _, unitType := range []SystemdUnitType{SystemdUnitTypeOCI, SystemdUnitTypeDebRpm} {
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
