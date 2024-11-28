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

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/envs"
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
