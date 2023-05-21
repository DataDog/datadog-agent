// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package languagedetection

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

func makeProcess(cmdline []string) *procutil.Process {
	return &procutil.Process{
		Pid:     rand.Int31(),
		Cmdline: cmdline,
		Stats: &procutil.Stats{
			CPUPercent: &procutil.CPUPercentStat{
				UserPct:   float64(rand.Uint64()),
				SystemPct: float64(rand.Uint64()),
			},
			MemInfo: &procutil.MemoryInfoStat{
				RSS: rand.Uint64(),
				VMS: rand.Uint64(),
			},
			MemInfoEx:   &procutil.MemoryInfoExStat{},
			IOStat:      &procutil.IOCountersStat{},
			CtxSwitches: &procutil.NumCtxSwitchesStat{},
		},
	}
}

func TestLanguageFromCommandline(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cmdline  []string
		expected LanguageName
		error    bool
	}{
		{
			name:     "python3",
			cmdline:  []string{"/opt/python/2.7.11/bin/python2.7", "/opt/foo/bar/baz", "--config=asdf"},
			expected: python,
		},
		{
			name:     "java",
			cmdline:  []string{"/usr/bin/java", "-Xfoo=true", "org.elasticsearch.bootstrap.Elasticsearch"},
			expected: java,
		},
		{
			name:     "unknown",
			cmdline:  []string{"mine-bitcoins", "--all"},
			expected: unknown,
			error:    true,
		},
		{
			name:     "python with space and special chars in path",
			cmdline:  []string{"//..//path/\"\\ to/python", "asdf"},
			expected: python,
		},
		{
			name:     "args in first element",
			cmdline:  []string{"/usr/bin/python myapp.py --config=/etc/mycfg.yaml"},
			expected: python,
		},
		{
			name:     "javac is not java",
			cmdline:  []string{"javac", "main.java"},
			expected: unknown,
		},
		{
			name:     "py is python",
			cmdline:  []string{"py", "test.py"},
			expected: python,
		},
		{
			name:     "py is not a prefix",
			cmdline:  []string{"pyret", "main.pyret"},
			expected: unknown,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			detected, err := languageNameFromCommandLine(tc.cmdline)
			assert.Equal(t, tc.expected, detected)
			if tc.error {
				assert.Error(t, err)
			}
		})
	}
}

func BenchmarkDetectLanguage(b *testing.B) {
	commands := [][]string{
		{"python", "--version"},
		{"python3", "--version"},
		{"py", "--version"},
		{"python", "-c", "import platform; print(platform.python_version())"},
		{"python3", "-c", "import platform; print(platform.python_version())"},
		{"py", "-c", "import platform; print(platform.python_version())"},
		{"python", "-c", "import sys; print(sys.version)"},
		{"python3", "-c", "import sys; print(sys.version)"},
		{"py", "-c", "import sys; print(sys.version)"},
		{"python", "-c", "print('Python')"},
		{"python3", "-c", "print('Python')"},
		{"py", "-c", "print('Python')"},
		{"java", "-version"},
		{"java", "-jar", "myapp.jar"},
		{"java", "-cp", ".", "MyClass"},
		{"javac", "MyClass.java"},
		{"javap", "-c", "MyClass"},
	}

	var procs []*procutil.Process
	for _, command := range commands {
		procs = append(procs, makeProcess(command))
	}

	b.StartTimer()
	DetectLanguage(procs)
}
