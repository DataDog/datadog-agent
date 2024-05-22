// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"go.uber.org/zap"

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
			require.Equal(t, tt.expected, isSpringBootArchive(&zip.Reader{File: tt.files}))
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
			cwd:        "/opt/somewhere/",
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
			cwd:        "/opt/somewhere/",
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
	parser := newSpringBootParser(NewDetectionContext(zap.NewNop(), nil, nil, fstest.MapFS(nil)))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsLocs, cpLocs := parser.parseURI(strings.Split(tt.locations, ";"), tt.configName, tt.profiles, tt.cwd)
			require.Equal(t, tt.expectedFileSystemLocations, fsLocs)
			require.Equal(t, tt.expectedClassPathLocations, cpLocs)
		})
	}
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
	parser := newSpringBootParser(NewDetectionContext(zap.NewNop(), nil, nil, &RealFs{}))

	fileSources := parser.scanSourcesFromFileSystem(map[string][]string{
		"fs": {
			abs("./application-fs.properties", cwd),
			abs("./*/application-fs.properties", cwd),
		},
	})
	require.Len(t, fileSources, 1)
	val, ok := fileSources["fs"]
	require.True(t, ok, "Expecting property source for fs profile")
	require.Equal(t, "from-fs", val.GetDefault("spring.application.name", "notfound"))
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

func createMockSpringBootApp(t *testing.T, destination io.Writer) {
	writer := zip.NewWriter(destination)
	require.NoError(t, writeFile(writer, "BOOT-INF/classes/application.properties", "spring.application.name=default-app"))
	require.NoError(t, writeFile(writer, "BOOT-INF/classes/config/prod/application-prod.properties", "spring.application.name=prod-app"))
	require.NoError(t, writeFile(writer, "BOOT-INF/classes/some/nested/location/application-yaml.yaml",
		`spring:
  application:
    name: yaml-app`))
	require.NoError(t, writeFile(writer, "BOOT-INF/classes/custom.properties", "spring.application.name=custom-app"))
	require.NoError(t, writer.Close())
}

func TestExtractServiceMetadataSpringBoot(t *testing.T) {
	filename := "app/app.jar"
	writer := bytes.NewBuffer(nil)
	createMockSpringBootApp(t, writer)
	fs := fstest.MapFS{
		filename: &fstest.MapFile{Data: writer.Bytes()},
	}
	tests := []struct {
		name     string
		jarname  string
		cmdline  []string
		envs     []string
		expected string
	}{
		{
			name:    "not a spring boot",
			jarname: "app.jar",
			cmdline: []string{
				"java",
				"-jar",
				"app.jar",
			},
			expected: "",
		},
		{
			name:    "spring boot with app name as arg",
			jarname: filename,
			cmdline: []string{
				"java",
				"-jar",
				filename,
				"--spring.application.name=found",
			},
			expected: "found",
		},
		{
			name:    "spring boot with app name as system property",
			jarname: filename,
			cmdline: []string{
				"java",
				"-Dspring.application.name=found",
				"-jar",
				filename,
			},
			expected: "found",
		},
		{
			name:    "spring boot with app name as env",
			jarname: filename,
			cmdline: []string{
				"java",
				"-jar",
				filename,
			},
			envs: []string{
				"SPRING_APPLICATION_NAME=found",
			},
			expected: "found",
		},
		{
			name:    "spring boot default options",
			jarname: filename,
			cmdline: []string{
				"java",
				"-jar",
				filename,
			},
			expected: "default-app",
		},
		{
			name:    "spring boot prod profile",
			jarname: filename,
			cmdline: []string{
				"java",
				"-Dspring.profiles.active=prod",
				"-jar",
				filename,
			},
			expected: "default-app",
		},
		{
			name:    "spring boot custom location",
			jarname: filename,
			cmdline: []string{
				"java",
				"-Dspring.config.locations=classpath:/**/location/",
				"-jar",
				filename,
				"--spring.profiles.active=yaml",
			},
			expected: "yaml-app",
		},
		{
			name:    "spring boot custom config name and non-matching profiles",
			jarname: filename,
			cmdline: []string{
				"java",
				"-Dspring.config.name=custom",
				"-jar",
				filename,
				"--spring.profiles.active=prod,yaml",
			},
			expected: "custom-app",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, ok := newSpringBootParser(NewDetectionContext(zap.NewNop(), tt.cmdline, tt.envs, fs)).GetSpringBootAppName(tt.jarname)
			require.Equal(t, tt.expected, app)
			require.Equal(t, len(app) > 0, ok)
		})
	}
}
