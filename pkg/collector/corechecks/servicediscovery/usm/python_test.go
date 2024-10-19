// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/envs"
)

func TestPythonDetect(t *testing.T) {
	//prepare the in mem fs
	memFs := fstest.MapFS{
		"modules/m1/first/nice/package/__init__.py": &fstest.MapFile{},
		"modules/m1/first/nice/__init__.py":         &fstest.MapFile{},
		"modules/m1/first/nice/something.py":        &fstest.MapFile{},
		"modules/m1/first/__init__.py":              &fstest.MapFile{},
		"modules/m1/__init__.py":                    &fstest.MapFile{},
		"modules/m2/":                               &fstest.MapFile{},
		"apps/app1/__main__.py":                     &fstest.MapFile{},
		"apps/app2/cmd/run.py":                      &fstest.MapFile{},
		"apps/app2/setup.py":                        &fstest.MapFile{},
	}
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, ok := newPythonDetector(NewDetectionContext(nil, envs.NewVariables(nil), memFs)).detect(strings.Split(tt.cmd, " ")[1:])
			require.Equal(t, tt.expected, value.Name)
			require.Equal(t, len(value.Name) > 0, ok)
		})
	}
}
