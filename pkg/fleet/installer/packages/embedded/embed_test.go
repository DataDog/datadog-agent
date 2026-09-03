// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package embedded

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
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
		units := embeddedUnitsInDir(t, "tmpl/gen/sd/"+string(unitType))
		assert.ElementsMatch(t, units, embeddedUnitsInDir(t, "tmpl/gen/sd/"+string(unitType)+"-nc"),
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

// TestGetLaunchdJobEmbedsBothVariants ensures every launchd job GetLaunchdJob can construct at
// runtime is actually compiled in, and that the definitions are property lists rather than
// whatever the template happened to emit.
func TestGetLaunchdJobEmbedsBothVariants(t *testing.T) {
	stable, err := LaunchdJobs(LaunchdStable)
	require.NoError(t, err)
	assert.NotEmpty(t, stable)
	assert.Contains(t, stable, "com.datadoghq.installer")
	assert.Contains(t, stable, "com.datadoghq.agent")

	experiment, err := LaunchdJobs(LaunchdExperiment)
	require.NoError(t, err)
	assert.NotEmpty(t, experiment)

	// The installer daemon supervises an experiment, so it is never part of one.
	assert.NotContains(t, experiment, "com.datadoghq.installer")

	for _, variant := range []LaunchdVariant{LaunchdStable, LaunchdExperiment} {
		labels, err := LaunchdJobs(variant)
		require.NoError(t, err)
		for _, label := range labels {
			t.Run(label+string(variant), func(t *testing.T) {
				content, err := GetLaunchdJob(label, variant)
				require.NoError(t, err)
				assert.NotEmpty(t, content)

				assertPropertyList(t, content)
				// RunAtLoad is what starts the job at boot with no login session, so it is
				// required on every definition rather than merely typical of them.
				assert.Contains(t, string(content), "<key>RunAtLoad</key>")
				assert.Contains(t, string(content), fmt.Sprintf("<string>%s</string>", label+string(variant)))
			})
		}
	}
}

// TestLaunchdExperimentJobsOmitKeepAlive is the property Fleet's failure detection rests on:
// launchd must never relaunch a failed experiment, so an -exp exit is a terminal event rather
// than one iteration of a respawn loop.
func TestLaunchdExperimentJobsOmitKeepAlive(t *testing.T) {
	labels, err := LaunchdJobs(LaunchdExperiment)
	require.NoError(t, err)
	require.NotEmpty(t, labels)

	for _, label := range labels {
		t.Run(label, func(t *testing.T) {
			experiment, err := GetLaunchdJob(label, LaunchdExperiment)
			require.NoError(t, err)
			assert.NotContains(t, string(experiment), "KeepAlive")

			// The stable definition of the same job is supervised, so the assertion above is
			// about the variant and not about the template having lost the key entirely.
			stable, err := GetLaunchdJob(label, LaunchdStable)
			require.NoError(t, err)
			assert.Contains(t, string(stable), "KeepAlive")
		})
	}
}

// TestLaunchdVariantsDifferOnlyAsSpecified asserts the four differences between the job sets that
// the RFC names, so a template edit cannot quietly collapse them.
func TestLaunchdVariantsDifferOnlyAsSpecified(t *testing.T) {
	stable, err := GetLaunchdJob("com.datadoghq.agent", LaunchdStable)
	require.NoError(t, err)
	experiment, err := GetLaunchdJob("com.datadoghq.agent", LaunchdExperiment)
	require.NoError(t, err)

	// 1. Label suffix.
	assert.Contains(t, string(stable), "<string>com.datadoghq.agent</string>")
	assert.Contains(t, string(experiment), "<string>com.datadoghq.agent-exp</string>")

	// 2. Configuration directory, named in the definition's own argv because launchd cannot
	//    supply one at load time.
	assert.Contains(t, string(stable), "<string>/opt/datadog-agent/etc</string>")
	assert.Contains(t, string(experiment), "<string>/opt/datadog-agent/etc-exp</string>")

	// 3. Restart supervision, covered in full by TestLaunchdExperimentJobsOmitKeepAlive.
	assert.Contains(t, string(stable), "KeepAlive")
	assert.NotContains(t, string(experiment), "KeepAlive")

	// Everything else is shared. The program above all: a configuration experiment changes what
	// the Agent reads, never which binary runs, so both sets name the one install root and write
	// the same pidfile in the same run directory.
	for _, content := range [][]byte{stable, experiment} {
		assert.Contains(t, string(content), "<string>/opt/datadog-agent/bin/agent/agent</string>")
		assert.Contains(t, string(content), "<string>/opt/datadog-agent/run/agent.pid</string>")
	}
}

// assertPropertyList checks that content is a well-formed XML property list. The repository has no
// plist decoder, and pulling one in for a structural check would be a dependency for one
// assertion; launchd will refuse a definition that is not well-formed XML rooted at <plist>, which
// is what this catches.
func assertPropertyList(t *testing.T, content []byte) {
	t.Helper()

	decoder := xml.NewDecoder(bytes.NewReader(content))
	var root string
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		require.NoError(t, err, "definition is not well-formed XML")
		if start, ok := token.(xml.StartElement); ok && root == "" {
			root = start.Name.Local
		}
	}
	assert.Equal(t, "plist", root, "definition is not rooted at <plist>")
}
