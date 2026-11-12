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

func TestGetSystemdUnitEmbedsPrivilegedRshellVariants(t *testing.T) {
	units := []string{
		"datadog-agent-rshell-privileged.service",
		"datadog-agent-rshell-privileged-exp.service",
		"datadog-agent-rshell-privileged.socket",
		"datadog-agent-rshell-privileged-exp.socket",
	}
	for _, unitType := range []UnitType{UnitTypeOCI, UnitTypeDebRpm} {
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

// launchdStableJobs are the launchd jobs the installer loads on macOS.
var launchdStableJobs = []string{
	"com.datadoghq.procmgr",
	"com.datadoghq.agent",
	"com.datadoghq.sysprobe",
	"com.datadoghq.data-plane",
	"com.datadoghq.installer",
}

// launchdExperimentJobs are the launchd jobs a configuration experiment swaps for their -exp
// variant. The process manager is never part of an experiment.
var launchdExperimentJobs = []string{
	"com.datadoghq.agent",
	"com.datadoghq.sysprobe",
	"com.datadoghq.data-plane",
	"com.datadoghq.installer",
}

// TestGetLaunchdJobEmbedsBothSets ensures every launchd job is actually compiled in, in both
// variants, and that the definitions are property lists rather than whatever the template
// happened to emit.
func TestGetLaunchdJobEmbedsBothSets(t *testing.T) {
	entries, err := launchdJobs.ReadDir("tmpl/gen/darwin")
	require.NoError(t, err)
	var embedded []string
	for _, entry := range entries {
		embedded = append(embedded, strings.TrimSuffix(entry.Name(), ".plist"))
	}
	var expected []string
	expected = append(expected, launchdStableJobs...)
	for _, label := range launchdExperimentJobs {
		expected = append(expected, label+string(LaunchdExperiment))
	}
	assert.ElementsMatch(t, expected, embedded)

	stable, err := LaunchdJobs(LaunchdStable)
	require.NoError(t, err)
	assert.ElementsMatch(t, launchdStableJobs, stable)
	experiment, err := LaunchdJobs(LaunchdExperiment)
	require.NoError(t, err)
	assert.ElementsMatch(t, launchdExperimentJobs, experiment)

	for _, label := range launchdStableJobs {
		t.Run(label, func(t *testing.T) {
			content, err := GetLaunchdJob(label, LaunchdStable)
			require.NoError(t, err)
			assert.NotEmpty(t, content)

			assertPropertyList(t, content)
			// RunAtLoad is what starts the job at boot with no login session, so it is
			// required on every stable definition rather than merely typical of them.
			assert.Regexp(t, `<key>RunAtLoad</key>\s*<true/>`, string(content))
			// Every stable job is supervised: launchd relaunches a job that exits unsuccessfully.
			assert.Contains(t, string(content), "KeepAlive")
			assert.Contains(t, string(content), fmt.Sprintf("<string>%s</string>", label))
		})
	}
	for _, label := range launchdExperimentJobs {
		t.Run(label+string(LaunchdExperiment), func(t *testing.T) {
			content, err := GetLaunchdJob(label, LaunchdExperiment)
			require.NoError(t, err)
			assertPropertyList(t, content)
			assert.Contains(t, string(content), fmt.Sprintf("<string>%s</string>", label+string(LaunchdExperiment)))
		})
	}
}

// TestLaunchdExperimentJobsAreNotSupervised is the property the config experiment's failure
// detection rests on: launchd must never start or relaunch an experiment job by itself, so an
// -exp exit is a terminal event rather than one iteration of a respawn loop, and a reboot
// mid-experiment brings back only the stable set.
func TestLaunchdExperimentJobsAreNotSupervised(t *testing.T) {
	for _, label := range launchdExperimentJobs {
		t.Run(label, func(t *testing.T) {
			experiment, err := GetLaunchdJob(label, LaunchdExperiment)
			require.NoError(t, err)
			assert.NotContains(t, string(experiment), "KeepAlive")
			assert.Regexp(t, `<key>RunAtLoad</key>\s*<false/>`, string(experiment))
		})
	}
	_, err := GetLaunchdJob("com.datadoghq.procmgr", LaunchdExperiment)
	assert.Error(t, err, "the process manager must have no experiment variant")
	installer, err := GetLaunchdJob("com.datadoghq.installer", LaunchdExperiment)
	require.NoError(t, err)
	assert.Regexp(t, `<key>DD_INSTALLER_EXPERIMENT_DAEMON</key>\s*<string>true</string>`, string(installer),
		"the experiment daemon must know it is one")
	stable, err := GetLaunchdJob("com.datadoghq.installer", LaunchdStable)
	require.NoError(t, err)
	assert.NotContains(t, string(stable), "DD_INSTALLER_EXPERIMENT_DAEMON")
}

// TestLaunchdExperimentJobsReadTheExperimentConfig pins that an experiment job reads etc-exp
// everywhere the stable job reads etc, and logs to its own file.
func TestLaunchdExperimentJobsReadTheExperimentConfig(t *testing.T) {
	for _, label := range launchdExperimentJobs {
		t.Run(label, func(t *testing.T) {
			stable, err := GetLaunchdJob(label, LaunchdStable)
			require.NoError(t, err)
			experiment, err := GetLaunchdJob(label, LaunchdExperiment)
			require.NoError(t, err)
			assert.NotContains(t, string(experiment), "/opt/datadog-agent/etc/")
			assert.NotContains(t, string(experiment), "<string>/opt/datadog-agent/etc</string>")
			assert.Contains(t, string(experiment), "/opt/datadog-agent/etc-exp")
			assert.Regexp(t, `<key>StandardOutPath</key>\s*<string>/opt/datadog-agent/logs/[a-z_]+-exp\.log</string>`, string(experiment))
			assert.NotContains(t, string(stable), "-exp")
		})
	}
	agent, err := GetLaunchdJob("com.datadoghq.agent", LaunchdExperiment)
	require.NoError(t, err)
	assert.Regexp(t, `<key>DD_CONFD_PATH</key>\s*<string>/opt/datadog-agent/etc-exp/conf.d</string>`, string(agent),
		"the Agent's default conf.d is under etc, so the experiment must name its own")
}

func TestLaunchdAgentAndSysprobeReadTheSameConfig(t *testing.T) {
	const etcDir = "/opt/datadog-agent/etc"
	for _, label := range []string{"com.datadoghq.agent", "com.datadoghq.sysprobe"} {
		content, err := GetLaunchdJob(label, LaunchdStable)
		require.NoError(t, err)
		var plist struct {
			Args []string `xml:"dict>array>string"`
		}
		require.NoError(t, xml.Unmarshal(content, &plist))
		require.GreaterOrEqual(t, len(plist.Args), 4)
		assert.Equal(t, []string{"run", "-c", etcDir}, plist.Args[1:4])
		if label == "com.datadoghq.agent" {
			require.GreaterOrEqual(t, len(plist.Args), 5)
			assert.Equal(t, "--sysprobecfgpath="+etcDir+"/system-probe.yaml", plist.Args[4],
				"the Agent loads system-probe config separately from its main config")
			// No pidfile: nothing reads it, and the Agent refuses to start when a stale one
			// names a PID that another process has since reused, which launchd's KeepAlive
			// would turn into a restart loop.
			assert.Len(t, plist.Args, 5)
			assert.NotContains(t, plist.Args, "-p")
		}
	}
}

// TestLaunchdInstallerJob pins the root installer daemon's job: it runs as root, logs next to the
// Agent's other jobs, leaves run_path to the configuration and writes its pidfile in the Agent's run
// directory, as on Linux.
func TestLaunchdInstallerJob(t *testing.T) {
	content, err := GetLaunchdJob("com.datadoghq.installer", LaunchdStable)
	require.NoError(t, err)
	definition := string(content)

	assert.NotContains(t, definition, "DD_RUN_PATH")
	assert.Regexp(t, `<key>StandardOutPath</key>\s*<string>/opt/datadog-agent/logs/launchd_installer.log</string>`, definition)
	assert.Regexp(t, `<key>StandardErrorPath</key>\s*<string>/opt/datadog-agent/logs/launchd_installer.log</string>`, definition)
	assert.Regexp(t, `<key>UserName</key>\s*<string>root</string>`, definition)

	var plist struct {
		Args []string `xml:"dict>array>string"`
	}
	require.NoError(t, xml.Unmarshal(content, &plist))
	assert.Equal(t, []string{"/opt/datadog-agent/embedded/bin/installer", "run", "-c", "/opt/datadog-agent/etc", "-p", "/opt/datadog-agent/run/installer.pid"}, plist.Args)
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

func TestShippedProcmgrConfigFilesListsEmbeds(t *testing.T) {
	shipped, err := ShippedProcmgrConfigFiles()
	require.NoError(t, err)
	require.NotEmpty(t, shipped)
	assert.Contains(t, shipped, "datadog-agent-ddot.yaml")
	assert.Contains(t, shipped, "datadog-agent-par-control.yaml")
}
