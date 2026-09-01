// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package launchd

import (
	"context"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
)

// swappableJobs are the labels defined in both variants. The installer daemon is deliberately
// absent: it is the process that performs the swap.
var swappableJobs = []string{
	"com.datadoghq.agent",
	"com.datadoghq.sysprobe",
	"com.datadoghq.data-plane",
}

func testJobSet(t *testing.T) (JobSet, *[][]string) {
	t.Helper()

	var calls [][]string
	client := NewClient(System)
	client.Runner = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		calls = append(calls, args)
		return nil, nil
	}
	return JobSet{Labels: swappableJobs, Dir: t.TempDir(), Client: client}, &calls
}

// plist is the subset of a launchd job definition these tests assert on.
type plist struct {
	label            string
	programArguments []string
	keys             map[string]bool
}

// parsePlist reads the top-level dictionary of a property list.
//
// The definitions are asserted against a parse rather than against their text, so a test cannot be
// satisfied by a string appearing in a comment and cannot be broken by reformatting.
func parsePlist(t *testing.T, content []byte) plist {
	t.Helper()

	parsed := plist{keys: map[string]bool{}}
	decoder := xml.NewDecoder(strings.NewReader(string(content)))
	decoder.Strict = false
	depth := 0
	var key string
	var collecting bool
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "dict", "array":
				depth++
				if depth == 2 && key == "ProgramArguments" {
					collecting = true
				}
			case "key":
				if depth == 1 {
					var name string
					require.NoError(t, decoder.DecodeElement(&name, &element))
					key = name
					parsed.keys[name] = true
				}
			case "string":
				var value string
				require.NoError(t, decoder.DecodeElement(&value, &element))
				switch {
				case collecting:
					parsed.programArguments = append(parsed.programArguments, value)
				case depth == 1 && key == "Label":
					parsed.label = value
				}
			}
		case xml.EndElement:
			if element.Name.Local == "dict" || element.Name.Local == "array" {
				if collecting && depth == 2 {
					collecting = false
				}
				depth--
			}
		}
	}
	return parsed
}

const (
	stableConfigDir     = "/opt/datadog-agent/etc"
	experimentConfigDir = "/opt/datadog-agent/etc-exp"
)

// TestTheTwoVariantsDifferInExactlyFourWays is the reason there are two job sets rather than one
// job restarted with different arguments. Each difference is what makes an experiment an
// experiment, so each is asserted against the definitions that actually ship.
func TestTheTwoVariantsDifferInExactlyFourWays(t *testing.T) {
	for _, label := range swappableJobs {
		t.Run(label, func(t *testing.T) {
			stableContent, err := embedded.GetLaunchdJob(label, Stable)
			require.NoError(t, err)
			experimentContent, err := embedded.GetLaunchdJob(label, Experiment)
			require.NoError(t, err)
			stable := parsePlist(t, stableContent)
			experiment := parsePlist(t, experimentContent)

			// 1. The label carries the suffix, so the two are distinct jobs to launchd and can
			//    never be loaded over one another.
			assert.Equal(t, label, stable.label)
			assert.Equal(t, label+"-exp", experiment.label)

			// 2. Each reads its own configuration directory. The jobs name it differently --
			//    the Agent takes -c and a directory, the data plane --config and a file -- so the
			//    assertion is that every argument naming etc names etc-exp in the experiment, and
			//    nothing else about the arguments changes.
			expected := make([]string, 0, len(stable.programArguments))
			var redirected int
			for _, argument := range stable.programArguments[1:] {
				if after, found := strings.CutPrefix(argument, stableConfigDir); found {
					argument = experimentConfigDir + after
					redirected++
				}
				expected = append(expected, argument)
			}
			assert.Positive(t, redirected, "no argument names the configuration directory")
			assert.Equal(t, expected, experiment.programArguments[1:])
			for _, argument := range stable.programArguments {
				assert.NotContains(t, argument, experimentConfigDir, "the stable job reads the experiment configuration")
			}

			// 3. The stable job runs through the façade, which is correct for every version
			//    forever; the experiment reaches directly into the pool's experiment link.
			require.NotEmpty(t, stable.programArguments)
			require.NotEmpty(t, experiment.programArguments)
			assert.True(t, strings.HasPrefix(stable.programArguments[0], "/opt/datadog-agent/"),
				"the stable job must name the façade, got %s", stable.programArguments[0])
			assert.True(t, strings.HasPrefix(experiment.programArguments[0], "/opt/datadog-packages/datadog-agent/experiment/"),
				"the experiment job must name the pool's experiment link, got %s", experiment.programArguments[0])
			assert.NotEqual(t, stable.programArguments[0], experiment.programArguments[0])

			// 4. The experiment has no KeepAlive. This is the difference that makes an experiment
			//    that exits terminal rather than one iteration of a respawn loop, which is what
			//    lets a failing experiment be observed and reverted instead of flapping forever.
			assert.True(t, stable.keys["KeepAlive"], "the stable job must be supervised")
			assert.False(t, experiment.keys["KeepAlive"], "the experiment job must not be respawned")
		})
	}
}

// TestTheInstallerHasNoExperimentVariant pins that the daemon driving an experiment is not part of
// the set being swapped.
func TestTheInstallerHasNoExperimentVariant(t *testing.T) {
	_, err := embedded.GetLaunchdJob("com.datadoghq.installer", Stable)
	require.NoError(t, err)
	_, err = embedded.GetLaunchdJob("com.datadoghq.installer", Experiment)
	assert.Error(t, err, "the installer must not have an experiment variant")

	labels, err := embedded.LaunchdJobs(Experiment)
	require.NoError(t, err)
	assert.ElementsMatch(t, swappableJobs, labels)
}

func TestJobSetWritesAndRemovesTheVariantDefinitions(t *testing.T) {
	jobs, _ := testJobSet(t)

	require.NoError(t, jobs.Write(Experiment))
	for _, label := range jobs.Labels {
		path := filepath.Join(jobs.Dir, label+"-exp.plist")
		content, err := os.ReadFile(path)
		require.NoError(t, err, "%s was not written", path)
		assert.Equal(t, label+"-exp", parsePlist(t, content).label)
		assert.NoFileExists(t, filepath.Join(jobs.Dir, label+".plist"), "writing one variant must not touch the other")
	}

	require.NoError(t, jobs.Remove(Experiment))
	for _, label := range jobs.Labels {
		assert.NoFileExists(t, filepath.Join(jobs.Dir, label+"-exp.plist"))
	}
	// Removing an absent definition succeeds: the hooks run on hosts that are already in the
	// desired state.
	require.NoError(t, jobs.Remove(Experiment))
}

func TestJobSetStartLoadsEnablesAndStartsEveryJob(t *testing.T) {
	jobs, calls := testJobSet(t)
	require.NoError(t, jobs.Write(Experiment))

	require.NoError(t, jobs.Start(context.Background(), Experiment))

	for _, label := range jobs.Labels {
		target := "system/" + label + "-exp"
		assert.Contains(t, *calls, []string{"bootstrap", "system", filepath.Join(jobs.Dir, label+"-exp.plist")})
		assert.Contains(t, *calls, []string{"enable", target})
		assert.Contains(t, *calls, []string{"kickstart", target})
	}
}

// TestJobSetStopUnloadsInReverse pins the teardown order: a job must not be left running against a
// dependency that has already gone away.
func TestJobSetStopUnloadsInReverse(t *testing.T) {
	jobs, calls := testJobSet(t)

	require.NoError(t, jobs.Stop(context.Background(), Stable))

	var bootedOut []string
	for _, args := range *calls {
		if len(args) == 2 && args[0] == "bootout" {
			bootedOut = append(bootedOut, args[1])
		}
	}
	require.Len(t, bootedOut, len(jobs.Labels))
	for i, label := range jobs.Labels {
		assert.Equal(t, "system/"+label, bootedOut[len(jobs.Labels)-1-i])
	}
}
