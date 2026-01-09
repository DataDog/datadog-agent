// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package usm

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/discovery/envs"
)

func TestPythonDetect(t *testing.T) {
	// Wrap the MapFS in a SubDirFS since we want to test absolute paths and the
	// former doesn't allow them in calls to Open().
	memFs := SubDirFS{FS: fstest.MapFS{
		"modules/m1/first/nice/package/__init__.py": &fstest.MapFile{},
		"modules/m1/first/nice/__init__.py":         &fstest.MapFile{},
		"modules/m1/first/nice/something.py":        &fstest.MapFile{},
		"modules/m1/first/__init__.py":              &fstest.MapFile{},
		"modules/m1/__init__.py":                    &fstest.MapFile{},
		"modules/m2/":                               &fstest.MapFile{},
		"apps/app1/__main__.py":                     &fstest.MapFile{},
		"apps/app2/cmd/run.py":                      &fstest.MapFile{},
		"apps/app2/setup.py":                        &fstest.MapFile{},
		"example.py":                                &fstest.MapFile{},
		"usr/bin/pytest":                            &fstest.MapFile{},
		"bin/WALinuxAgent.egg":                      &fstest.MapFile{},
	}}
	tests := []struct {
		name     string
		cmd      string
		expected string
	}{
		{
			name:     "3rd level module path",
			cmd:      "python modules/m1/first/nice/package",
			expected: "m1.first.nice.package",
		},
		{
			name:     "2nd level module path",
			cmd:      "python modules/m1/first/nice",
			expected: "m1.first.nice",
		},
		{
			name:     "2nd level explicit script inside module",
			cmd:      "python modules/m1/first/nice/something.py",
			expected: "m1.first.nice.something",
		},
		{
			name:     "1st level module path",
			cmd:      "python modules/m1/first",
			expected: "m1.first",
		},
		{
			name:     "empty module",
			cmd:      "python modules/m2",
			expected: "m2",
		},
		{
			name:     "__main__ in a dir ",
			cmd:      "python apps/app1",
			expected: "app1",
		},
		{
			name:     "script in inner dir ",
			cmd:      "python apps/app2/cmd/run.py",
			expected: "app2",
		},
		{
			name:     "script in top level dir ",
			cmd:      "python apps/app2/setup.py",
			expected: "app2",
		},
		{
			name:     "top level script",
			cmd:      "python example.py",
			expected: "example",
		},
		{
			name:     "root level script",
			cmd:      "python /example.py",
			expected: "example",
		},
		{
			name: "root level script with ..",
			// This results in a path of "." after findNearestTopLevel is called on the split path.
			cmd:      "python /../example.py",
			expected: "example",
		},
		{
			name:     "script in bin",
			cmd:      "python /usr/bin/pytest",
			expected: "pytest",
		},
		{
			name:     "script in bin with -u",
			cmd:      "python3 -u bin/WALinuxAgent.egg",
			expected: "WALinuxAgent",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, ok := newPythonDetector(NewDetectionContext(nil, envs.NewVariables(nil), memFs)).detect(strings.Split(tt.cmd, " ")[1:])
			require.Equal(t, tt.expected, value.Name)
			require.Equal(t, len(value.Name) > 0, ok)
		})
	}
}

func TestParsePythonArgs(t *testing.T) {
	tests := []struct {
		args         []string
		expectedType argType
		expectedArg  string
	}{
		{
			args:         []string{},
			expectedType: argNone,
			expectedArg:  "",
		},
		{
			args:         []string{"-"},
			expectedType: argNone,
			expectedArg:  "",
		},
		{
			args:         []string{"-B"},
			expectedType: argNone,
			expectedArg:  "",
		},
		{
			args:         []string{"-X"},
			expectedType: argNone,
			expectedArg:  "",
		},
		{
			args:         []string{""},
			expectedType: argFileName,
			expectedArg:  "",
		},
		{
			args:         []string{"script.py"},
			expectedType: argFileName,
			expectedArg:  "script.py",
		},
		{
			args:         []string{"-u", "script.py"},
			expectedType: argFileName,
			expectedArg:  "script.py",
		},
		{
			args:         []string{"-B", "script.py"},
			expectedType: argFileName,
			expectedArg:  "script.py",
		},
		{
			args:         []string{"-E", "script.py"},
			expectedType: argFileName,
			expectedArg:  "script.py",
		},
		{
			args:         []string{"-OO", "script.py"},
			expectedType: argFileName,
			expectedArg:  "script.py",
		},
		{
			args:         []string{"script.py", "-OO"},
			expectedType: argFileName,
			expectedArg:  "script.py",
		},
		{
			args:         []string{"-u", "-B", "-E", "script.py"},
			expectedType: argFileName,
			expectedArg:  "script.py",
		},
		{
			args:         []string{"-W", "default", "app.py"},
			expectedType: argFileName,
			expectedArg:  "app.py",
		},
		{
			args:         []string{"-Wdefault", "app.py"},
			expectedType: argFileName,
			expectedArg:  "app.py",
		},
		{
			args:         []string{"-c", "print('hello')", "app.py"},
			expectedType: argNone,
			expectedArg:  "",
		},
		{
			args:         []string{"-cprint('hello')", "app.py"},
			expectedType: argNone,
			expectedArg:  "",
		},
		{
			args:         []string{"-m", "module", "app.py"},
			expectedType: argMod,
			expectedArg:  "module",
		},
		{
			args:         []string{"-mmodule", "app.py"},
			expectedType: argMod,
			expectedArg:  "module",
		},
		{
			args:         []string{"-BEmmodule", "-u", "script.py"},
			expectedType: argMod,
			expectedArg:  "module",
		},
		{
			args:         []string{"--check-hash-based-pycs", "always", "app.py"},
			expectedType: argFileName,
			expectedArg:  "app.py",
		},
		{
			args:         []string{"--foo", "script.py"},
			expectedType: argFileName,
			expectedArg:  "script.py",
		},
		{
			args:         []string{"-BE", "script.py"},
			expectedType: argFileName,
			expectedArg:  "script.py",
		},
		{
			args:         []string{"-BEs", "-u", "script.py"},
			expectedType: argFileName,
			expectedArg:  "script.py",
		},
		{
			args:         []string{"-BW", "ignore", "script.py"},
			expectedType: argFileName,
			expectedArg:  "script.py",
		},
		{
			args:         []string{"-X", "dev", "script.py"},
			expectedType: argFileName,
			expectedArg:  "script.py",
		},
		{
			args:         []string{"-Xdev", "script.py"},
			expectedType: argFileName,
			expectedArg:  "script.py",
		},
		{
			args:         []string{"-W", "error::DeprecationWarning", "script.py"},
			expectedType: argFileName,
			expectedArg:  "script.py",
		},
		{
			args:         []string{"-Werror::DeprecationWarning", "script.py"},
			expectedType: argFileName,
			expectedArg:  "script.py",
		},
	}

	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			argType, arg := parsePythonArgs(tt.args)
			require.Equal(t, tt.expectedType, argType)
			require.Equal(t, tt.expectedArg, arg)
		})
	}
}

func TestGunicornCmdline(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		expected string
	}{
		{
			name:     "simple module:app pattern",
			cmd:      "gunicorn myapp:app",
			expected: "myapp",
		},
		{
			name:     "module:app with parentheses",
			cmd:      "gunicorn FOOd:create_app()",
			expected: "FOOd",
		},
		{
			name:     "module:app with complex arguments",
			cmd:      "gunicorn foo.middleware:make_app(config_path='/foo/web.ini') --bind 0.0.0.0:5555 --workers 4 --bind 127.0.0.1:5123 --workers 4 --worker-class eventlet --keep-alive 300 --timeout 120 --graceful-timeout 60 --log-config /etc/log.ini --statsd-host 10.2.1.1:8125 --statsd-prefix foo.web",
			expected: "foo.middleware",
		},
		{
			name:     "app name not last argument",
			cmd:      "gunicorn myapp:app --bind 0.0.0.0:8000 --workers 1",
			expected: "myapp",
		},
		{
			name:     "app name after no arg flag",
			cmd:      "gunicorn --worker-class foo --daemon myapp",
			expected: "myapp",
		},
		{
			name:     "with name flag",
			cmd:      "gunicorn -n myapp",
			expected: "myapp",
		},
		{
			name:     "with name flag and value",
			cmd:      "gunicorn --name=myapp",
			expected: "myapp",
		},
		{
			name:     "with name flag and attached value",
			cmd:      "gunicorn -nmyapp",
			expected: "myapp",
		},
		{
			name:     "with name flag and attached value with other flag",
			cmd:      "gunicorn -Rnmyapp",
			expected: "myapp",
		},
		{
			name:     "with other flag",
			cmd:      "gunicorn -unfake myapp",
			expected: "myapp",
		},
		{
			name:     "with name flag and other flags",
			cmd:      "gunicorn -R -nmyapp --bind 0.0.0.0:8000",
			expected: "myapp",
		},
		{
			name:     "with name flag and other flags reversed",
			cmd:      "gunicorn --bind 0.0.0.0:8000 -nmyapp -R",
			expected: "myapp",
		},
		{
			name:     "with name flag and module:app pattern",
			cmd:      "gunicorn -nmyapp test:app",
			expected: "myapp",
		},
		{
			name:     "with name flag and module:app pattern reversed",
			cmd:      "gunicorn test:app -nmyapp",
			expected: "myapp",
		},
		{
			name:     "no app name found",
			cmd:      "gunicorn --bind 0.0.0.0:8000",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, ok := extractGunicornNameFrom(strings.Split(tt.cmd, " ")[1:])
			require.Equal(t, tt.expected, value)
			require.Equal(t, len(value) > 0, ok)
		})
	}
}
