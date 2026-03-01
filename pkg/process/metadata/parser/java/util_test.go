// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package javaparser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestArgumentPropertySource(t *testing.T) {
	argSlice := []string{"-c",
		"-Dspring.application.name=test",
		"--spring.profiles.active=prod",
		"--d",
		"-Ddefined.something",
	}
	tests := []struct {
		name     string
		prefix   string
		args     []string
		expected map[string]string
	}{
		{
			name:   "should parse spring boot app args",
			prefix: "--",
			args:   argSlice,
			expected: map[string]string{
				"spring.profiles.active": "prod",
				"d":                      "",
			},
		},
		{
			name:   "should parse system properties",
			prefix: "-D",
			args:   argSlice,
			expected: map[string]string{
				"spring.application.name": "test",
				"defined.something":       "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			argSource := newArgumentSource(tt.args, tt.prefix)
			for key, expected := range tt.expected {
				value, ok := argSource.Get(key)
				require.True(t, ok)
				require.Equal(t, expected, value)
			}
		})
	}
}
func TestScanSourcesFromFileSystem(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	fileSources := scanSourcesFromFileSystem(map[string][]string{
		"fs": {
			filepath.ToSlash(abs("./application-fs.properties", cwd)),
			filepath.ToSlash(abs("./*/application-fs.properties", cwd)),
		},
	})
	require.Len(t, fileSources, 1)
	val, ok := fileSources["fs"]
	if !ok {
		require.Fail(t, "Expecting property source for fs profile")
	} else {
		require.Equal(t, "from-fs", val.GetDefault("spring.application.name", "notfound"))
	}
}

func TestLongestPathPrefix(t *testing.T) {
	tests := []struct {
		pattern  string
		expected string
	}{
		{"/test/**/*.xml", "/test"},
		{"/foo/bar/baz.yml", "/foo/bar/baz.yml"},
		{"**/*.properties", ""},
		{"/config/*.yaml", "/config"},
		{"?.yaml", ""},
		{"/a/b/c/**/d/*.properties", "/a/b/c"},
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			require.Equal(t, tt.expected, longestPathPrefix(tt.pattern))
		})
	}
}

func TestMapSourceGetDefault(t *testing.T) {
	src := &mapSource{m: map[string]string{"key1": "val1"}}

	val := src.GetDefault("key1", "default")
	require.Equal(t, "val1", val)

	val = src.GetDefault("missing", "default")
	require.Equal(t, "default", val)
}

func TestNewPropertySourceFromFile(t *testing.T) {
	// Non-existent file should return error
	_, err := newPropertySourceFromFile("/nonexistent/path/file.properties")
	require.Error(t, err)

	// Valid properties file
	tmpFile := filepath.Join(t.TempDir(), "test.properties")
	err = os.WriteFile(tmpFile, []byte("spring.application.name=test-app\nserver.port=8080"), 0644)
	require.NoError(t, err)

	src, err := newPropertySourceFromFile(tmpFile)
	require.NoError(t, err)
	require.NotNil(t, src)

	val, ok := src.Get("spring.application.name")
	require.True(t, ok)
	require.Equal(t, "test-app", val)
}

func TestNewPropertySourceFromStream(t *testing.T) {
	tests := []struct {
		name          string
		filename      string
		errorExpected bool
		filesize      uint64
	}{
		{
			name:          "should not be case sensitive to file extensions",
			filename:      "test.YAmL",
			errorExpected: false,
		},
		{
			name:          "should allow properties files",
			filename:      "test.properties",
			errorExpected: false,
			filesize:      maxParseFileSize,
		},
		{
			name:          "should allow also yml files",
			filename:      "TEST.YML",
			errorExpected: false,
		},
		{
			name:          "should return an error for unhandled file formats",
			filename:      "unknown.extension",
			errorExpected: true,
		},
		{
			name:          "should not parse files larger than MiB",
			filename:      "large.yaml",
			errorExpected: true,
			filesize:      maxParseFileSize + 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := newPropertySourceFromStream(strings.NewReader(" "), tt.filename, tt.filesize)
			if tt.errorExpected {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, value)
			}
		})
	}
}
