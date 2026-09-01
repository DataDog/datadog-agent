// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubJobDir points the job definitions at a temporary directory.
func stubJobDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	original := launchdJobDir
	launchdJobDir = dir
	t.Cleanup(func() { launchdJobDir = original })
	return dir
}

// launchctlCalls flattens the recorded invocations to "verb target" pairs, in order.
func launchctlCalls(calls [][]string, verb string) []string {
	var targets []string
	for _, args := range calls {
		if len(args) >= 2 && args[0] == verb {
			targets = append(targets, args[len(args)-1])
		}
	}
	return targets
}

// TestStartConfigExperimentHandsOverToTheExperimentSet pins the order of the swap: the stable jobs
// must be unloaded before the experiment ones are loaded, or two Agents would run at once against
// the same PID files and the same intake.
func TestStartConfigExperimentHandsOverToTheExperimentSet(t *testing.T) {
	calls := stubLaunchd(t)
	dir := stubJobDir(t)

	require.NoError(t, postStartConfigExperimentDatadogAgent(testHookContext(t)))

	for _, label := range agentJobs {
		content, err := os.ReadFile(filepath.Join(dir, label+"-exp.plist"))
		require.NoError(t, err, "the experiment definition for %s was not written", label)
		assert.Contains(t, string(content), "<string>"+label+"-exp</string>")
		assert.NoFileExists(t, filepath.Join(dir, label+".plist"), "the stable definition was rewritten")
	}

	bootedOut := launchctlCalls(*calls, "bootout")
	bootstrapped := launchctlCalls(*calls, "bootstrap")
	require.NotEmpty(t, bootedOut)
	require.NotEmpty(t, bootstrapped)
	for _, label := range agentJobs {
		assert.Contains(t, bootedOut, "system/"+label)
		assert.Contains(t, bootstrapped, filepath.Join(dir, label+"-exp.plist"))
	}

	// The installer daemon is the process running this hook. Stopping it would abandon the
	// experiment it has just started, with nothing left to revert it.
	for _, args := range *calls {
		for _, argument := range args {
			assert.NotContains(t, argument, installerJob, "the swap touched the installer daemon: %v", args)
		}
	}

	// Every stable job is unloaded before any experiment job is loaded.
	lastStableBootout, firstExperimentStart := -1, len(*calls)
	for i, args := range *calls {
		joined := strings.Join(args, " ")
		if args[0] == "bootout" && !strings.Contains(joined, "-exp") {
			lastStableBootout = i
		}
		if (args[0] == "bootstrap" || args[0] == "kickstart") && strings.Contains(joined, "-exp") && i < firstExperimentStart {
			firstExperimentStart = i
		}
	}
	assert.Less(t, lastStableBootout, firstExperimentStart, "an experiment job was loaded while a stable one was still running")
}

// TestStopConfigExperimentRestoresTheStableSet is the revert path. It must leave the host with no
// trace of the experiment, including no experiment definitions on disk.
func TestStopConfigExperimentRestoresTheStableSet(t *testing.T) {
	calls := stubLaunchd(t)
	dir := stubJobDir(t)
	require.NoError(t, postStartConfigExperimentDatadogAgent(testHookContext(t)))
	*calls = nil

	require.NoError(t, preStopConfigExperimentDatadogAgent(testHookContext(t)))

	for _, label := range agentJobs {
		assert.NoFileExists(t, filepath.Join(dir, label+"-exp.plist"), "an experiment definition survived the revert")
		content, err := os.ReadFile(filepath.Join(dir, label+".plist"))
		require.NoError(t, err, "the stable definition for %s was not restored", label)
		assert.Contains(t, string(content), "<string>"+label+"</string>")
	}

	bootedOut := launchctlCalls(*calls, "bootout")
	for _, label := range agentJobs {
		assert.Contains(t, bootedOut, "system/"+label+"-exp")
	}
	for _, label := range agentJobs {
		assert.Contains(t, launchctlCalls(*calls, "enable"), "system/"+label)
		assert.Contains(t, launchctlCalls(*calls, "kickstart"), "system/"+label)
	}
}

// TestPromoteConfigExperimentRestartsTheStableSet covers the other outcome. The stable jobs are
// started fresh so they read the configuration the installer has just promoted; a job left loaded
// would keep serving the pre-promotion configuration it was started with.
func TestPromoteConfigExperimentRestartsTheStableSet(t *testing.T) {
	calls := stubLaunchd(t)
	dir := stubJobDir(t)
	require.NoError(t, postStartConfigExperimentDatadogAgent(testHookContext(t)))
	*calls = nil

	require.NoError(t, postPromoteConfigExperimentDatadogAgent(testHookContext(t)))

	for _, label := range agentJobs {
		assert.NoFileExists(t, filepath.Join(dir, label+"-exp.plist"))
		assert.Contains(t, launchctlCalls(*calls, "bootout"), "system/"+label+"-exp")
		assert.Contains(t, launchctlCalls(*calls, "kickstart"), "system/"+label)
	}
}

// TestExperimentJobsMatchTheEmbeddedDefinitions guards the two label lists against drifting apart
// from the definitions that ship: a job missing from agentJobs would never be swapped, and would
// keep running the stable configuration for the whole life of an experiment.
func TestExperimentJobsMatchTheEmbeddedDefinitions(t *testing.T) {
	assert.NotContains(t, agentJobs, installerJob)
	assert.Contains(t, stableJobs, installerJob)
	assert.Len(t, stableJobs, len(agentJobs)+1)

	for i, label := range agentJobs {
		assert.Equal(t, label+"-exp", experimentJobs[i])
	}
	assert.Len(t, experimentJobs, len(agentJobs))
}
