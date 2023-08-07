// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package languagedetection

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

func makeProcess(cmdline []string) *procutil.Process {
	return &procutil.Process{
		Pid:     rand.Int31(),
		Cmdline: cmdline,
	}
}

func TestLanguageFromCommandline(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cmdline  []string
		expected languagemodels.LanguageName
	}{
		{
			name:     "python2",
			cmdline:  []string{"/opt/Python/2.7.11/bin/python2.7", "/opt/foo/bar/baz", "--config=asdf"},
			expected: languagemodels.Python,
		},
		{
			name:     "Java",
			cmdline:  []string{"/usr/bin/Java", "-Xfoo=true", "org.elasticsearch.bootstrap.Elasticsearch"},
			expected: languagemodels.Java,
		},
		{
			name:     "Unknown",
			cmdline:  []string{"mine-bitcoins", "--all"},
			expected: languagemodels.Unknown,
		},
		{
			name:     "Python with space and special chars in path",
			cmdline:  []string{"//..//path/\"\\ to/Python", "asdf"},
			expected: languagemodels.Python,
		},
		{
			name:     "args in first element",
			cmdline:  []string{"/usr/bin/Python myapp.py --config=/etc/mycfg.yaml"},
			expected: languagemodels.Python,
		},
		{
			name:     "javac is not Java",
			cmdline:  []string{"javac", "main.Java"},
			expected: languagemodels.Unknown,
		},
		{
			name:     "py is Python",
			cmdline:  []string{"py", "test.py"},
			expected: languagemodels.Python,
		},
		{
			name:     "py is not a prefix",
			cmdline:  []string{"pyret", "main.pyret"},
			expected: languagemodels.Unknown,
		},
		{
			name:     "node",
			cmdline:  []string{"node", "/etc/app/index.js"},
			expected: languagemodels.Node,
		},
		{
			name:     "npm",
			cmdline:  []string{"npm", "start"},
			expected: languagemodels.Node,
		},
		{
			name:     "dotnet",
			cmdline:  []string{"dotnet", "myApp"},
			expected: languagemodels.Dotnet,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, languageNameFromCommandLine(tc.cmdline))
		})
	}
}

func TestGetExe(t *testing.T) {
	type test struct {
		name     string
		cmdline  []string
		expected string
	}

	for _, tc := range []test{
		{
			name:     "blank",
			cmdline:  []string{},
			expected: "",
		},
		{
			name:     "python",
			cmdline:  []string{"/usr/bin/python", "test.py"},
			expected: "python",
		},
		{
			name:     "numeric ending",
			cmdline:  []string{"/usr/bin/python3.9", "test.py"},
			expected: "python3.9",
		},
		{
			name:     "packed args",
			cmdline:  []string{"java -jar Test.jar"},
			expected: "java",
		},
		{
			name:     "uppercase",
			cmdline:  []string{"/usr/bin/MyBinary"},
			expected: "mybinary",
		},
		{
			name:     "dont trim .exe on linux",
			cmdline:  []string{"/usr/bin/helloWorld.exe"},
			expected: "helloworld.exe",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, getExe(tc.cmdline))
		})
	}
}

func BenchmarkDetectLanguage(b *testing.B) {
	commands := [][]string{
		{"Python", "--version"},
		{"python3", "--version"},
		{"py", "--version"},
		{"Python", "-c", "import platform; print(platform.python_version())"},
		{"python3", "-c", "import platform; print(platform.python_version())"},
		{"py", "-c", "import platform; print(platform.python_version())"},
		{"Python", "-c", "import sys; print(sys.version)"},
		{"python3", "-c", "import sys; print(sys.version)"},
		{"py", "-c", "import sys; print(sys.version)"},
		{"Python", "-c", "print('Python')"},
		{"python3", "-c", "print('Python')"},
		{"py", "-c", "print('Python')"},
		{"Java", "-version"},
		{"Java", "-jar", "myapp.jar"},
		{"Java", "-cp", ".", "MyClass"},
		{"javac", "MyClass.Java"},
		{"javap", "-c", "MyClass"},
	}

	var procs []*procutil.Process
	for _, command := range commands {
		procs = append(procs, makeProcess(command))
	}

	b.StartTimer()

	for i := 0; i < b.N; i++ {
		DetectLanguage(procs, nil)
	}
}
