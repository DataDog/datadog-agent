// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package apm

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/usm"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	usmtestutil "github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
)

func TestInjected(t *testing.T) {
	data := []struct {
		name   string
		envs   map[string]string
		result bool
	}{
		{
			name: "injected",
			envs: map[string]string{
				"DD_INJECTION_ENABLED": "tracer",
			},
			result: true,
		},
		{
			name: "one of injected",
			envs: map[string]string{
				"DD_INJECTION_ENABLED": "service_name,tracer",
			},
			result: true,
		},
		{
			name: "not injected but with env variable",
			envs: map[string]string{
				"DD_INJECTION_ENABLED": "service_name",
			},
		},
		{
			name: "not injected, no env variable",
		},
	}
	for _, d := range data {
		t.Run(d.name, func(t *testing.T) {
			result := isInjected(d.envs)
			assert.Equal(t, d.result, result)
		})
	}
}

func Test_javaDetector(t *testing.T) {
	data := []struct {
		name   string
		args   []string
		envs   map[string]string
		result Instrumentation
	}{
		{
			name:   "not there",
			args:   strings.Split("java -jar Foo.jar Foo", " "),
			result: None,
		},
		{
			name:   "version",
			args:   strings.Split("java -version", " "),
			result: None,
		},
		{
			name:   "cmdline",
			args:   []string{"java", "-foo", "-javaagent:/path/to/data dog/dd-java-agent.jar", "-Ddd.profiling.enabled=true"},
			result: Provided,
		},
		{
			name: "CATALINA_OPTS",
			args: []string{"java"},
			envs: map[string]string{
				"CATALINA_OPTS": "-javaagent:dd-java-agent.jar",
			},
			result: Provided,
		},
	}
	for _, d := range data {
		t.Run(d.name, func(t *testing.T) {
			result := javaDetector(0, d.args, d.envs, nil)
			if result != d.result {
				t.Errorf("expected %s got %s", d.result, result)
			}
		})
	}
}

func Test_nodeDetector(t *testing.T) {
	curDir, err := testutil.CurDir()
	assert.NoError(t, err)

	data := []struct {
		name       string
		contextMap usm.DetectorContextMap
		result     Instrumentation
	}{
		{
			name: "not instrumented",
			contextMap: usm.DetectorContextMap{
				usm.NodePackageJSONPath: filepath.Join(curDir, "testdata/node/not_instrumented/package.json"),
				usm.ServiceSubFS:        usm.NewSubDirFS("/"),
			},
			result: None,
		},
		{
			name: "instrumented",
			contextMap: usm.DetectorContextMap{
				usm.NodePackageJSONPath: filepath.Join(curDir, "testdata/node/instrumented/package.json"),
				usm.ServiceSubFS:        usm.NewSubDirFS("/"),
			},
			result: Provided,
		},
	}

	for _, d := range data {
		t.Run(d.name, func(t *testing.T) {
			result := nodeDetector(0, nil, nil, d.contextMap)
			assert.Equal(t, d.result, result)
		})
	}
}

func Test_pythonDetector(t *testing.T) {
	data := []struct {
		name   string
		maps   string
		result Instrumentation
	}{
		{
			name:   "empty maps",
			maps:   "",
			result: None,
		},
		{
			name: "not in maps",
			maps: `
79f6cd47d000-79f6cd47f000 r--p 00000000 fc:04 793163                     /usr/lib/python3.10/lib-dynload/_bz2.cpython-310-x86_64-linux-gnu.so
79f6cd479000-79f6cd47a000 r-xp 00001000 fc:06 5507018                    /home/foo/.local/lib/python3.10/site-packages/ddtrace_fake/md.cpython-310-x86_64-linux-gnu.so
			`,
			result: None,
		},
		{
			name: "in maps",
			maps: `
79f6cd47d000-79f6cd47f000 r--p 00000000 fc:04 793163                     /usr/lib/python3.10/lib-dynload/_bz2.cpython-310-x86_64-linux-gnu.so
79f6cd482000-79f6cd484000 r--p 00005000 fc:04 793163                     /usr/lib/python3.10/lib-dynload/_bz2.cpython-310-x86_64-linux-gnu.so
79f6cd438000-79f6cd441000 r-xp 00004000 fc:06 7895596                    /home/foo/.local/lib/python3.10/site-packages-internal/ddtrace/internal/datadog/profiling/crashtracker/_crashtracker.cpython-310-x86_64-linux-gnu.so
			`,
			result: Provided,
		},
	}
	for _, d := range data {
		t.Run(d.name, func(t *testing.T) {
			result := pythonDetectorFromMapsReader(strings.NewReader(d.maps))
			assert.Equal(t, d.result, result)
		})
	}
}

func TestGoDetector(t *testing.T) {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)
	serverBinWithSymbols, err := usmtestutil.BuildGoBinaryWrapper(filepath.Join(curDir, "testutil"), "instrumented")
	require.NoError(t, err)
	serverBinWithoutSymbols, err := usmtestutil.BuildGoBinaryWrapperWithoutSymbols(filepath.Join(curDir, "testutil"), "instrumented")
	require.NoError(t, err)

	cmdWithSymbols := exec.Command(serverBinWithSymbols)
	require.NoError(t, cmdWithSymbols.Start())
	t.Cleanup(func() {
		_ = cmdWithSymbols.Process.Kill()
	})

	cmdWithoutSymbols := exec.Command(serverBinWithoutSymbols)
	require.NoError(t, cmdWithoutSymbols.Start())
	t.Cleanup(func() {
		_ = cmdWithoutSymbols.Process.Kill()
	})

	result := goDetector(os.Getpid(), nil, nil, nil)
	require.Equal(t, None, result)

	result = goDetector(cmdWithSymbols.Process.Pid, nil, nil, nil)
	require.Equal(t, Provided, result)

	result = goDetector(cmdWithoutSymbols.Process.Pid, nil, nil, nil)
	require.Equal(t, Provided, result)
}
