// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package usm

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func TestGetClassPath(t *testing.T) {
	tests := []struct {
		args     []string
		expected []string
	}{
		{
			args:     []string{},
			expected: []string{"."},
		},
		{
			args:     []string{"java"},
			expected: []string{"."},
		},
		{
			args:     []string{"java", "-a", "-cp", "foo"},
			expected: []string{"foo"},
		},
		{
			args:     []string{"java", "-a", "-cp", "first", "-cp", "second"},
			expected: []string{"second"},
		},
		{
			args:     []string{"java", "-a", "-cp", "first", "-cp", "second", "-jar", "jar", "-cp", "ignore"},
			expected: []string{"second"},
		},
		{
			args:     []string{"java", "-cp", ""},
			expected: []string{"."},
		},
		{
			args:     []string{"java", "-x", "-classpath", "bar:x:y"},
			expected: []string{"bar", "x", "y"},
		},
		{
			args:     []string{"java", "-x", "--class-path", ":baz"},
			expected: []string{"", "baz"},
		},
		{
			args:     []string{"java", "-x", "--class-path=quux:"},
			expected: []string{"quux", ""},
		},
		{
			args:     []string{"java", "-something:somearg", "-cp", "real"},
			expected: []string{"real"},
		},
		{
			args:     []string{"java", "--something=somearg", "-cp", "real"},
			expected: []string{"real"},
		},
		{
			args:     []string{"java", "--something", "somearg", "-cp", "real"},
			expected: []string{"real"},
		},
		{
			args:     []string{"java", "someclass", "-cp", "fake"},
			expected: []string{"."},
		},
		{
			args:     []string{"java", "-jar", "jarname", "-cp", "fake"},
			expected: []string{"."},
		},
		{
			args:     []string{"java", "-m", "modname", "-cp", "fake"},
			expected: []string{"."},
		},
		{
			args:     []string{"java", "-cp", "real", "--module", "modname", "-cp", "fake"},
			expected: []string{"real"},
		},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			value := getClassPath(tt.args)
			require.Equal(t, tt.expected, value)
		})
	}
}

func TestFoo(t *testing.T) {
	tests := []struct {
		data     string
		expected string
	}{
		{
			data:     "foo",
			expected: "",
		},
		{
			data: `
Start-Class:  `,
			expected: "",
		},
		{
			data: `
Foo: x
Start-Class: org.corp.App
Bar: blah
`,
			expected: "org.corp.App",
		},
		{
			data: strings.Repeat("A", maxParseFileSize) +
				`
Start-Class: org.corp.App
`,
			expected: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.data[:min(len(tt.data), 10)], func(t *testing.T) {
			fs := fstest.MapFS{
				"MANIFEST.MF": &fstest.MapFile{Data: []byte(tt.data)},
			}
			value, err := getStartClassName(fs, "MANIFEST.MF")
			if tt.expected == "" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, value)
			}
		})
	}
}
