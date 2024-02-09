// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package languagedetection

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
)

func TestDetectLanguage(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cmdline  []string
		comm     string
		expected languagemodels.LanguageName
	}{
		{
			name:     "python2",
			cmdline:  []string{"/opt/Python/2.7.11/bin/python2.7", "/opt/foo/bar/baz", "--config=asdf"},
			comm:     "baz",
			expected: languagemodels.Python,
		},
		{
			name:     "Java",
			cmdline:  []string{"/usr/bin/Java", "-Xfoo=true", "org.elasticsearch.bootstrap.Elasticsearch"},
			comm:     "java",
			expected: languagemodels.Java,
		},
		{
			name:     "Unknown",
			cmdline:  []string{"mine-bitcoins", "--all"},
			comm:     "mine-bitcoins",
			expected: languagemodels.Unknown,
		},
		{
			name:     "Python with space and special chars in path",
			cmdline:  []string{"//..//path/\"\\ to/Python", "asdf"},
			comm:     "asdf",
			expected: languagemodels.Python,
		},
		{
			name:     "args in first element",
			cmdline:  []string{"/usr/bin/Python myapp.py --config=/etc/mycfg.yaml"},
			comm:     "myapp.py",
			expected: languagemodels.Python,
		},
		{
			name:     "javac is not Java",
			cmdline:  []string{"javac", "main.Java"},
			comm:     "javac",
			expected: languagemodels.Unknown,
		},
		{
			name:     "py is Python",
			cmdline:  []string{"py", "test.py"},
			comm:     "test.py",
			expected: languagemodels.Python,
		},
		{
			name:     "py is not a prefix",
			cmdline:  []string{"pyret", "main.pyret"},
			comm:     "pyret",
			expected: languagemodels.Unknown,
		},
		{
			name:     "node",
			cmdline:  []string{"node", "/etc/app/index.js"},
			comm:     "node",
			expected: languagemodels.Node,
		},
		{
			name:     "npm",
			cmdline:  []string{"npm", "start"},
			comm:     "npm",
			expected: languagemodels.Node,
		},
		{
			name:     "dotnet",
			cmdline:  []string{"dotnet", "myApp"},
			comm:     "dotnet",
			expected: languagemodels.Dotnet,
		},
		{
			name:     "ruby",
			cmdline:  []string{"ruby", "prog.rb"},
			comm:     "ruby",
			expected: languagemodels.Ruby,
		},
		{
			name:     "rails",
			cmdline:  []string{"puma", "5.6.6", "(tcp://localhost:3000)", "[app]"},
			comm:     "ruby",
			expected: languagemodels.Ruby,
		},
		{
			name:     "irb",
			cmdline:  []string{"irb"},
			comm:     "ruby2.7",
			expected: languagemodels.Ruby,
		},
		{
			name:     "jruby",
			cmdline:  []string{"java", "-Djruby.home=/usr/share/jruby", "-Djruby.lib=/usr/share/jruby/lib", "org.jruby.Main", "prog.rb"},
			comm:     "java",
			expected: languagemodels.Ruby,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			process := []languagemodels.Process{makeProcess(tc.cmdline, tc.comm)}
			expected := []*languagemodels.Language{{Name: tc.expected}}
			assert.Equal(t, expected, DetectLanguage(process, nil))
		})
	}
}

func TestDetectLanguageBulk(t *testing.T) {
	cmds := []struct {
		cmdline  []string
		comm     string
		expected languagemodels.LanguageName
	}{
		{
			cmdline:  []string{"java", "-Djruby.home=/usr/share/jruby", "-Djruby.lib=/usr/share/jruby/lib", "org.jruby.Main", "prog.rb"},
			comm:     "java",
			expected: languagemodels.Ruby,
		},
		{
			cmdline:  []string{"node", "/etc/app/index.js"},
			comm:     "node",
			expected: languagemodels.Node,
		},
		{
			cmdline:  []string{"pyret", "main.pyret"},
			comm:     "pyret",
			expected: languagemodels.Unknown,
		},
		{
			cmdline:  []string{"/usr/bin/Java", "-Xfoo=true", "org.elasticsearch.bootstrap.Elasticsearch"},
			comm:     "java",
			expected: languagemodels.Java,
		},
	}

	procs := make([]languagemodels.Process, 0, len(cmds))
	for _, cmd := range cmds {
		procs = append(procs, makeProcess(cmd.cmdline, cmd.comm))
	}

	langs := DetectLanguage(procs, nil)
	for i, lang := range langs {
		assert.Equal(t, cmds[i].expected, lang.Name)
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
	commands := []struct {
		cmdline []string
		comm    string
	}{
		{
			cmdline: []string{"Python", "--version"},
			comm:    "",
		},
		{
			cmdline: []string{"python3", "--version"},
			comm:    "",
		},
		{
			cmdline: []string{"py", "--version"},
			comm:    "",
		},
		{
			cmdline: []string{"Python", "-c", "import platform; print(platform.python_version())"},
			comm:    "",
		},
		{
			cmdline: []string{"python3", "-c", "import platform; print(platform.python_version())"},
			comm:    "",
		},
		{
			cmdline: []string{"py", "-c", "import platform; print(platform.python_version())"},
			comm:    "",
		},
		{
			cmdline: []string{"Python", "-c", "import sys; print(sys.version)"},
			comm:    "",
		},
		{
			cmdline: []string{"python3", "-c", "import sys; print(sys.version)"},
			comm:    "",
		},
		{
			cmdline: []string{"py", "-c", "import sys; print(sys.version)"},
			comm:    "",
		},
		{
			cmdline: []string{"Python", "-c", "print('Python')"},
			comm:    "",
		},
		{
			cmdline: []string{"python3", "-c", "print('Python')"},
			comm:    "",
		},
		{
			cmdline: []string{"py", "-c", "print('Python')"},
			comm:    "",
		},
		{
			cmdline: []string{"Java", "-version"},
			comm:    "",
		},
		{
			cmdline: []string{"Java", "-jar", "myapp.jar"},
			comm:    "",
		},
		{
			cmdline: []string{"Java", "-cp", ".", "MyClass"},
			comm:    "",
		},
		{
			cmdline: []string{"javac", "MyClass.Java"},
			comm:    "",
		},
		{
			cmdline: []string{"javap", "-c", "MyClass"},
			comm:    "",
		},
		{
			cmdline: []string{"ruby", "prog.rb"},
			comm:    "ruby",
		},
		{
			cmdline: []string{"puma", "5.6.6", "(tcp://localhost:3000)", "[app]"},
			comm:    "ruby",
		},
		{
			cmdline: []string{"irb"},
			comm:    "ruby2.7",
		},
	}

	var procs []languagemodels.Process
	for _, command := range commands {
		procs = append(procs, makeProcess(command.cmdline, command.comm))
	}

	b.StartTimer()

	for i := 0; i < b.N; i++ {
		DetectLanguage(procs, nil)
	}
}
