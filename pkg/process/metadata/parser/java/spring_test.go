// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package javaparser

import (
	"archive/zip"
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsSpringBootArchive(t *testing.T) {
	tests := []struct {
		name     string
		files    []*zip.File
		expected bool
	}{
		{
			name: "contains a BOOT-INF directory on top level",
			files: []*zip.File{
				{FileHeader: zip.FileHeader{Name: "MANIFEST.MF"}},
				{FileHeader: zip.FileHeader{Name: "BOOT-INF/"}},
			},
			expected: true,
		},
		{
			name: "contains a BOOT-INF file on top level",
			files: []*zip.File{
				{FileHeader: zip.FileHeader{Name: "BOOT-INF"}},
			},
			expected: false,
		},
		{
			name: "contains a BOOT-INF directory on a nested level",
			files: []*zip.File{
				{FileHeader: zip.FileHeader{Name: "MANIFEST.MF"}},
				{FileHeader: zip.FileHeader{Name: "META-INF/BOOT-INF/"}},
			},
			expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &zip.Reader{File: tt.files}
			require.Equal(t, tt.expected, IsSpringBootArchive(reader))
		})
	}
}

func TestParseUri(t *testing.T) {
	tests := []struct {
		name                        string
		locations                   string
		configName                  string
		profiles                    []string
		cwd                         string
		expectedFileSystemLocations map[string][]string
		expectedClassPathLocations  map[string][]string
	}{
		{
			name:       "should parse the spring boot defaults",
			locations:  defaultLocations,
			configName: defaultConfigName,
			profiles:   nil,
			cwd:        filepath.FromSlash("/opt/somewhere/"),
			expectedFileSystemLocations: map[string][]string{
				"": {
					"/opt/somewhere/application.properties",
					"/opt/somewhere/application.yaml",
					"/opt/somewhere/application.yml",
					"/opt/somewhere/config/application.properties",
					"/opt/somewhere/config/application.yaml",
					"/opt/somewhere/config/application.yml",
					"/opt/somewhere/config/*/application.properties",
					"/opt/somewhere/config/*/application.yaml",
					"/opt/somewhere/config/*/application.yml",
				},
			},
			expectedClassPathLocations: map[string][]string{
				"": {
					"BOOT-INF/classes/application.properties",
					"BOOT-INF/classes/application.yaml",
					"BOOT-INF/classes/application.yml",
					"BOOT-INF/classes/config/application.properties",
					"BOOT-INF/classes/config/application.yaml",
					"BOOT-INF/classes/config/application.yml",
				},
			},
		},
		{
			name:       "should handle profiles and direct files",
			locations:  "file:/opt/anotherdir/anotherfile.properties;file:./",
			configName: "custom",
			profiles:   []string{"prod"},
			cwd:        filepath.FromSlash("/opt/somewhere/"),
			expectedFileSystemLocations: map[string][]string{
				"prod": {
					"/opt/somewhere/custom-prod.properties",
					"/opt/somewhere/custom-prod.yaml",
					"/opt/somewhere/custom-prod.yml",
				},
				"": {
					"/opt/anotherdir/anotherfile.properties",
					"/opt/somewhere/custom.properties",
					"/opt/somewhere/custom.yaml",
					"/opt/somewhere/custom.yml",
				},
			},
			expectedClassPathLocations: map[string][]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsLocs, cpLocs := parseURI(strings.Split(tt.locations, ";"), tt.configName, tt.profiles, tt.cwd)
			require.Equal(t, tt.expectedFileSystemLocations, fsLocs)
			require.Equal(t, tt.expectedClassPathLocations, cpLocs)
		})
	}
}

func writeFile(writer *zip.Writer, name string, content string) error {
	w, err := writer.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(content))
	return err
}

func TestNewSpringBootArchiveSourceFromReader(t *testing.T) {
	// create a test jar
	buf := bytes.NewBuffer([]byte{})
	writer := zip.NewWriter(buf)
	require.NoError(t, writeFile(writer, "BOOT-INF/classes/application.properties", "spring.application.name=default"))
	require.NoError(t, writeFile(writer, "BOOT-INF/classes/config/prod/application-prod.properties", "spring.application.name=prod"))
	require.NoError(t, writer.Close())
	tests := []struct {
		name     string
		patterns []string
		expected string
	}{
		{
			name:     "should find default application.properties",
			patterns: []string{"BOOT-INF/classes/application.properties", "BOOT-INF/classes/config/*/application.properties"},
			expected: "default",
		},
		{
			name: "should find prod application.properties",
			patterns: []string{
				"BOOT-INF/classes/application-prod.properties",
				"BOOT-INF/classes/config/*/application-prod.properties",
			},
			expected: "prod",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
			require.NoError(t, err)
			props := newSpringBootArchiveSourceFromReader(reader, map[string][]string{"": tt.patterns})
			require.Len(t, props, 1)
			require.NotNil(t, props[""])
			require.Equal(t, tt.expected, props[""].GetDefault("spring.application.name", "unknown"))
		})
	}
}
