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

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/envs"
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
			result := isInjected(envs.NewVariables(d.envs))
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
			name:   "cmdline - dd-java-agent.jar",
			args:   []string{"java", "-foo", "-javaagent:/path/to/data dog/dd-java-agent.jar", "-Ddd.profiling.enabled=true"},
			result: Provided,
		},
		{
			name:   "cmdline - dd-trace-agent.jar",
			args:   []string{"java", "-foo", "-javaagent:/path/to/data dog/dd-trace-agent.jar", "-Ddd.profiling.enabled=true"},
			result: Provided,
		},
		{
			name:   "cmdline - datadog.jar",
			args:   []string{"java", "-foo", "-javaagent:/path/to/data dog/datadog.jar", "-Ddd.profiling.enabled=true"},
			result: Provided,
		},
		{
			name:   "cmdline - datadog only in does not match",
			args:   []string{"java", "-foo", "path/to/data dog/datadog", "-Ddd.profiling.enabled=true"},
			result: None,
		},
		{
			name:   "cmdline - jar only in does not match",
			args:   []string{"java", "-foo", "path/to/data dog/datadog.jar", "-Ddd.profiling.enabled=true"},
			result: None,
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
			ctx := usm.NewDetectionContext(d.args, envs.NewVariables(d.envs), nil)
			result := javaDetector(ctx)
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
			ctx := usm.NewDetectionContext(nil, envs.NewVariables(nil), nil)
			ctx.ContextMap = d.contextMap
			result := nodeDetector(ctx)
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

func TestDotNetDetector(t *testing.T) {
	for _, test := range []struct {
		name   string
		envs   map[string]string
		maps   string
		result Instrumentation
	}{
		{
			name:   "no env, no maps",
			result: None,
		},
		{
			name: "profiling disabled",
			envs: map[string]string{
				"CORECLR_ENABLE_PROFILING": "0",
			},
			result: None,
		},
		{
			name: "profiling enabled",
			envs: map[string]string{
				"CORECLR_ENABLE_PROFILING": "1",
			},
			result: Provided,
		},
		{
			name: "not in maps",
			maps: `
785c8ab24000-785c8ab2c000 r--s 00000000 fc:06 12762114                   /home/foo/hello/bin/release/net8.0/linux-x64/publish/System.Diagnostics.StackTrace.dll
785c8ab2c000-785c8acce000 r--s 00000000 fc:06 12762148                   /home/foo/hello/bin/release/net8.0/linux-x64/publish/System.Net.Http.dll
			`,
			result: None,
		},
		{
			name: "in maps, no env",
			maps: `
785c89c00000-785c8a400000 rw-p 00000000 00:00 0
785c8a400000-785c8aaeb000 r--s 00000000 fc:06 12762267                   /home/foo/hello/bin/release/net8.0/linux-x64/publish/Datadog.Trace.dll
785c8aaec000-785c8ab0d000 rw-p 00000000 00:00 0
785c8ab0d000-785c8ab24000 r--s 00000000 fc:06 12761829                   /home/foo/hello/bin/release/net8.0/linux-x64/publish/System.Collections.Specialized.dll
			`,
			result: Provided,
		},
		{
			name: "in maps, env misleading",
			envs: map[string]string{
				"CORECLR_ENABLE_PROFILING": "0",
			},
			maps: `
785c8a400000-785c8aaeb000 r--s 00000000 fc:06 12762267                   /home/foo/hello/bin/release/net8.0/linux-x64/publish/Datadog.Trace.dll
			`,
			result: Provided,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var result Instrumentation
			if test.maps == "" {
				ctx := usm.NewDetectionContext(nil, envs.NewVariables(test.envs), nil)
				result = dotNetDetector(ctx)
			} else {
				result = dotNetDetectorFromMapsReader(strings.NewReader(test.maps))
			}
			assert.Equal(t, test.result, result)
		})
	}
}

func TestGoDetector(t *testing.T) {
	if os.Getenv("CI") == "" && os.Getuid() != 0 {
		t.Skip("skipping test; requires root privileges")
	}
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
	ctx := usm.NewDetectionContext(nil, envs.NewVariables(nil), nil)
	ctx.Pid = os.Getpid()
	result := goDetector(ctx)
	require.Equal(t, None, result)

	ctx.Pid = cmdWithSymbols.Process.Pid
	result = goDetector(ctx)
	require.Equal(t, Provided, result)

	ctx.Pid = cmdWithoutSymbols.Process.Pid
	result = goDetector(ctx)
	require.Equal(t, Provided, result)
}
