// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package flare

import (
	"archive/zip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	extensiontypes "github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/types"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// testAPIKey looks like a real API key so that the scrubber recognizes it
const testAPIKey = "0123456789abcdef0123456789abcdef"

func newGlobalParamsTest(t *testing.T) *subcommands.GlobalParams {
	tmpDir := t.TempDir()
	t.Setenv("TEST_FLARE_API_KEY", testAPIKey)

	// Create a simple OTel configuration file
	configPath := path.Join(tmpDir, "otel-config.yaml")
	configContent := `receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

exporters:
  datadog:
    api:
      key: ${env:TEST_FLARE_API_KEY}

extensions:
  health_check:
    endpoint: localhost:13133
  pprof:
    endpoint: localhost:1777
  zpages:
    endpoint: localhost:55679

service:
  extensions: [health_check, pprof, zpages]
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [datadog]
    metrics:
      receivers: [otlp]
      exporters: [datadog]
    logs:
      receivers: [otlp]
      exporters: [datadog]
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	return &subcommands.GlobalParams{
		ConfPaths: []string{"file:" + configPath},
	}
}

// newUnreachableCollector returns a config and an IPC client for which no collector is running,
// so the flare falls back to the configuration files.
func newUnreachableCollector(t *testing.T) (config.Component, *ipcmock.IPCMock) {
	cfg := configmock.New(t)
	cfg.SetInTest("otelcollector.extension_url", "https://127.0.0.1:1")
	return cfg, ipcmock.New(t)
}

// readZip returns the content of every file of a zip archive
func readZip(t *testing.T, zipPath string) map[string][]byte {
	zipReader, err := zip.OpenReader(zipPath)
	require.NoError(t, err)
	defer zipReader.Close()

	files := map[string][]byte{}
	for _, f := range zipReader.File {
		rc, err := f.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(rc)
		rc.Close()
		require.NoError(t, err)
		files[f.Name] = content
	}
	return files
}

func TestCreateOTelFlare(t *testing.T) {
	globalParams := newGlobalParamsTest(t)
	cfg, ipcComp := newUnreachableCollector(t)

	// Create the flare
	flarePath, err := createOTelFlare(globalParams, cfg, ipcComp.GetClient())
	require.NoError(t, err, "createOTelFlare should succeed")
	require.NotEmpty(t, flarePath, "flare path should not be empty")

	// Verify the file exists
	_, err = os.Stat(flarePath)
	require.NoError(t, err, "flare file should exist")

	// Clean up
	defer os.Remove(flarePath)

	// Verify the zip file contains expected files
	zipReader, err := zip.OpenReader(flarePath)
	require.NoError(t, err, "should be able to open zip file")
	defer zipReader.Close()

	expectedFiles := map[string]bool{
		"otel-response.json":          false,
		"build-info.txt":              false,
		"config/customer-config.yaml": false,
		"config/env-config.yaml":      false,
		"config/runtime-config.yaml":  false,
		"environment.json":            false,
		"debug-sources.json":          false,
	}

	for _, file := range zipReader.File {
		if _, exists := expectedFiles[file.Name]; exists {
			expectedFiles[file.Name] = true
		}
	}

	// Verify all expected files are present
	for fileName, found := range expectedFiles {
		assert.True(t, found, "Expected file %s not found in flare archive", fileName)
	}

	// The API key, resolved from the environment, must be scrubbed everywhere
	for name, content := range readZip(t, flarePath) {
		assert.NotContains(t, string(content), testAPIKey, "API key not scrubbed from %s", name)
	}
}

func TestCreateOTelFlareFromRunningCollector(t *testing.T) {
	globalParams := newGlobalParamsTest(t)

	profile := []byte("\x00\x01binary pprof " + testAPIKey)
	debugServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/debug/pprof/heap":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(profile)
		case "/debug/servicez":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html>api_key: " + testAPIKey + "</html>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer debugServer.Close()
	debugHost := strings.TrimPrefix(debugServer.URL, "http://")

	response := extensiontypes.Response{
		BuildInfoResponse: extensiontypes.BuildInfoResponse{AgentCommand: "otel-agent", ExtensionVersion: "running-collector"},
		ConfigResponse: extensiontypes.ConfigResponse{
			CustomerConfig: "exporters:\n  datadog:\n    api:\n      key: ${env:DD_API_KEY}\n",
			RuntimeConfig:  "exporters:\n  datadog:\n    api:\n      key: " + testAPIKey + "\nextensions:\n  zpages/dd-autoconfigured: {}\n",
		},
		DebugSourceResponse: extensiontypes.DebugSourceResponse{
			Sources: map[string]extensiontypes.OTelFlareSource{
				"pprof":                    {URLs: []string{debugHost + "/debug/pprof/heap"}},
				"zpages/dd-autoconfigured": {URLs: []string{debugHost + "/debug/servicez", debugHost + "/debug/missing"}},
			},
		},
		Environment: map[string]string{"DD_API_KEY": testAPIKey},
	}
	responseBytes, err := json.Marshal(response)
	require.NoError(t, err)

	ipcComp := ipcmock.New(t)
	extensionServer := ipcComp.NewMockServer(ipcComp.HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(responseBytes)
	})))
	cfg := configmock.New(t)
	cfg.SetInTest("otelcollector.extension_url", extensionServer.URL)

	flarePath, err := createOTelFlare(globalParams, cfg, ipcComp.GetClient())
	require.NoError(t, err)
	defer os.RemoveAll(filepath.Dir(flarePath))

	files := readZip(t, flarePath)

	// The effective configuration of the running collector is used, not the configuration files
	assert.Contains(t, string(files["build-info.txt"]), "Extension Version: running-collector")
	assert.Contains(t, string(files["config/runtime-config.yaml"]), "zpages/dd-autoconfigured")

	// Debug sources are fetched, including the ones added by the converter
	assert.Equal(t, profile, files["otel-flare/pprof_debug_pprof_heap.dat"], "binary profiles must be kept as is")
	assert.Contains(t, string(files["otel-flare/zpages/dd-autoconfigured_debug_servicez.dat"]), "<html>")
	assert.Contains(t, string(files["otel-flare/zpages/dd-autoconfigured_debug_missing.err"]), "404")

	for name, content := range files {
		if name == "otel-flare/pprof_debug_pprof_heap.dat" {
			continue
		}
		assert.NotContains(t, string(content), testAPIKey, "API key not scrubbed from %s", name)
	}
}

// TestScrubResponseStructuredSecrets checks secrets that the text scrubber alone misses once
// serialized: quoted JSON keys and YAML embedded in JSON strings.
func TestScrubResponseStructuredSecrets(t *testing.T) {
	data := &extensiontypes.Response{
		ConfigResponse: extensiontypes.ConfigResponse{
			CustomerConfig: noConfigProvided,
			RuntimeConfig:  "exporters:\n  otlphttp:\n    headers:\n      password: hunter2\nreceivers:\n  otlp: {}\n",
		},
		Environment: map[string]string{"AWS_SECRET_ACCESS_KEY": "supersecret", "HOME": "/root"},
	}

	scrubbed, err := scrubResponse(data)
	require.NoError(t, err)

	tmp := filepath.Join(t.TempDir(), "flare.zip")
	require.NoError(t, createFlareArchive(tmp, responseFiles(scrubbed)))
	files := readZip(t, tmp)
	for name, content := range files {
		assert.NotContains(t, string(content), "supersecret", "secret not scrubbed from %s", name)
		assert.NotContains(t, string(content), "hunter2", "secret not scrubbed from %s", name)
	}
	assert.Contains(t, string(files["environment.json"]), "/root", "non-secret values are kept")
	assert.Contains(t, string(files["config/runtime-config.yaml"]), "otlp", "non-secret config is kept")
	assert.Equal(t, noConfigProvided, string(files["config/customer-config.yaml"]))

	// the input is not modified
	assert.Equal(t, "supersecret", data.Environment["AWS_SECRET_ACCESS_KEY"])
}

func TestFlareConfigUsesExporterAPIKey(t *testing.T) {
	globalParams := newGlobalParamsTest(t)
	// flareConfigModule rebuilds the global config: the mock restores it after the test
	configmock.New(t)

	var cfg config.Component
	app := fx.New(flareConfigModule(globalParams), fx.Populate(&cfg))
	require.NoError(t, app.Err())

	assert.Equal(t, testAPIKey, cfg.GetString("api_key"))
}

func TestCollectOTelData(t *testing.T) {
	globalParams := newGlobalParamsTest(t)

	// Collect OTel data
	data, err := collectConfigData(globalParams)
	require.NoError(t, err, "collectOTelData should succeed")
	require.NotNil(t, data, "collected data should not be nil")

	// Verify basic response fields
	assert.Equal(t, "otel-agent", data.AgentCommand)
	assert.Equal(t, "Datadog OTel Agent", data.AgentDesc)
	assert.NotEmpty(t, data.AgentVersion, "agent version should not be empty")

	// Verify configs are populated
	assert.NotEmpty(t, data.RuntimeConfig, "runtime config should not be empty")
	assert.NotEmpty(t, data.EnvConfig, "env config should not be empty")

	// Verify environment is populated
	assert.NotEmpty(t, data.Environment, "environment should not be empty")

	// Verify debug sources are discovered
	assert.NotNil(t, data.Sources, "debug sources should not be nil")

	// Check for expected debug extensions
	expectedExtensions := []string{"health_check", "pprof", "zpages"}
	for _, ext := range expectedExtensions {
		source, exists := data.Sources[ext]
		assert.True(t, exists, "Expected debug extension %s not found", ext)
		if exists {
			assert.NotEmpty(t, source.URLs, "Debug extension %s should have URLs", ext)
		}
	}
}

func TestDiscoverDebugSources(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test configuration with debug extensions
	configPath := filepath.Join(tmpDir, "test-config.yaml")
	configContent := `extensions:
  health_check:
    endpoint: localhost:13133
  pprof:
    endpoint: localhost:1777
  zpages:
    endpoint: localhost:55679
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Discover debug sources
	sources := discoverDebugSources(context.TODO(), []string{"file:" + configPath})

	// Verify all three extensions are discovered
	assert.Contains(t, sources, "health_check", "health_check should be discovered")
	assert.Contains(t, sources, "pprof", "pprof should be discovered")
	assert.Contains(t, sources, "zpages", "zpages should be discovered")

	// Verify health_check URLs
	if healthCheck, ok := sources["health_check"]; ok {
		assert.Len(t, healthCheck.URLs, 1, "health_check should have 1 URL")
		assert.Contains(t, healthCheck.URLs[0], "http://localhost:13133")
	}

	// Verify pprof URLs
	if pprof, ok := sources["pprof"]; ok {
		assert.Len(t, pprof.URLs, 3, "pprof should have 3 URLs")
		assert.Contains(t, pprof.URLs[0], "http://localhost:1777/debug/pprof/heap")
		assert.Contains(t, pprof.URLs[1], "http://localhost:1777/debug/pprof/allocs")
		assert.Contains(t, pprof.URLs[2], "http://localhost:1777/debug/pprof/profile")
	}

	// Verify zpages URLs
	if zpages, ok := sources["zpages"]; ok {
		assert.Len(t, zpages.URLs, 5, "zpages should have 5 URLs")
		assert.Contains(t, zpages.URLs[0], "http://localhost:55679/debug/servicez")
		assert.Contains(t, zpages.URLs[1], "http://localhost:55679/debug/pipelinez")
		assert.Contains(t, zpages.URLs[2], "http://localhost:55679/debug/extensionz")
		assert.Contains(t, zpages.URLs[3], "http://localhost:55679/debug/featurez")
		assert.Contains(t, zpages.URLs[4], "http://localhost:55679/debug/tracez")
	}
}

func TestExtractExtensionType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple extension name",
			input:    "health_check",
			expected: "health_check",
		},
		{
			name:     "extension with instance name",
			input:    "pprof/dd-autoconfigured",
			expected: "pprof",
		},
		{
			name:     "extension with multiple slashes",
			input:    "zpages/custom/instance",
			expected: "zpages",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractExtensionType(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCreateOTelFlareUsesUnpredictableDir ensures each flare gets its own unpredictable
// directory, not a predictable path in the shared (world-writable) temp dir.
func TestCreateOTelFlareUsesUnpredictableDir(t *testing.T) {
	globalParams := newGlobalParamsTest(t)

	cfg, ipcComp := newUnreachableCollector(t)

	p1, err := createOTelFlare(globalParams, cfg, ipcComp.GetClient())
	require.NoError(t, err)
	defer os.RemoveAll(filepath.Dir(p1))

	p2, err := createOTelFlare(globalParams, cfg, ipcComp.GetClient())
	require.NoError(t, err)
	defer os.RemoveAll(filepath.Dir(p2))

	assert.NotEqual(t, filepath.Dir(p1), filepath.Dir(p2),
		"each flare must use a unique directory (os.MkdirTemp), not a predictable shared path")
}

// TestCreateOTelFlareDirectoryIsPrivate checks the flare dir is private (0700 on Unix) so others can't
// read the archive or pre-seed a symlink. Bit check is Unix-only; Windows inherits the per-user temp ACL.
func TestCreateOTelFlareDirectoryIsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not meaningful on Windows; the directory inherits the caller's private per-user temp ACL")
	}

	globalParams := newGlobalParamsTest(t)
	cfg, ipcComp := newUnreachableCollector(t)

	flarePath, err := createOTelFlare(globalParams, cfg, ipcComp.GetClient())
	require.NoError(t, err)
	defer os.RemoveAll(filepath.Dir(flarePath))

	dirInfo, err := os.Stat(filepath.Dir(flarePath))
	require.NoError(t, err)
	assert.Zerof(t, dirInfo.Mode().Perm()&0o077,
		"flare directory must not be accessible by group/other, got %o", dirInfo.Mode().Perm())
}

func TestFlareCommand(t *testing.T) {
	globalConfGetter := func() *subcommands.GlobalParams {
		return newGlobalParamsTest(t)
	}
	fxutil.TestOneShotSubcommand(t,
		[]*cobra.Command{MakeCommand(globalConfGetter)},
		[]string{"flare", "--send"},
		makeFlare,
		func() {},
	)
}
