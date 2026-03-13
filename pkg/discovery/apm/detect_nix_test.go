// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package apm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/discovery/envs"
	"github.com/DataDog/datadog-agent/pkg/discovery/usm"
)

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
