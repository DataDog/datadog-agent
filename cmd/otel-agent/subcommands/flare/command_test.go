// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package flare

import (
	"archive/zip"
	"bytes"
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

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	extensiontypes "github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/types"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// testAPIKey looks like a real API key so that the scrubber recognizes it
const testAPIKey = "0123456789abcdef0123456789abcdef"

// testSessionCookie is a secret that no scrubber recognizes, by its value or by the name of its variable
const testSessionCookie = "c0ffee-cafe-1234-abcd-0123456789"

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
	// Keep the tests off the log file of a collector installed on the machine
	cfg.SetInTest("log_file", filepath.Join(t.TempDir(), "otel-agent.log"))
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
	t.Setenv("DD_HOSTNAME", "flare-test-host")
	t.Setenv("SESSION_COOKIE", testSessionCookie)
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

	files := readZip(t, flarePath)

	// The reason of the fallback is kept in the archive, not only printed
	assert.Contains(t, string(files["otel-agent.log"]), "did not get otel-agent configuration")
	assert.Contains(t, string(files["otel-agent.log"]), "won't contain the effective configuration")

	// Only the environment variables that are safe to share are kept: a secret can have any name
	assert.Contains(t, string(files["environment.json"]), "DD_HOSTNAME")

	// The API key, resolved from the environment, must be scrubbed everywhere
	for name, content := range files {
		assert.NotContains(t, string(content), testAPIKey, "API key not scrubbed from %s", name)
		assert.NotContains(t, string(content), testSessionCookie, "session cookie kept in %s", name)
	}
}

// TestCollectOTelDataFallbackHint checks that a flare that can't reach the collector says how to find
// its auth token, the usual cause when the collector is running, on the console and in the archive.
func TestCollectOTelDataFallbackHint(t *testing.T) {
	globalParams := newGlobalParamsTest(t)
	cfg, ipcComp := newUnreachableCollector(t)

	var output bytes.Buffer
	previousOutput := color.Output
	color.Output = &output
	t.Cleanup(func() { color.Output = previousOutput })

	_, reason, err := collectOTelData(globalParams, cfg, ipcComp.GetClient())
	require.NoError(t, err)

	assert.Contains(t, output.String(), "DD_AUTH_TOKEN_FILE_PATH")
	assert.Contains(t, reason, "DD_AUTH_TOKEN_FILE_PATH")
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
		Environment: map[string]string{"DD_API_KEY": testAPIKey, "DD_SITE": "datadoghq.eu", "SESSION_COOKIE": testSessionCookie},
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
	logFile := filepath.Join(t.TempDir(), "otel-agent.log")
	require.NoError(t, os.WriteFile(logFile, []byte("log of the running collector"), 0644))
	cfg.SetInTest("log_file", logFile)

	flarePath, err := createOTelFlare(globalParams, cfg, ipcComp.GetClient())
	require.NoError(t, err)
	defer os.RemoveAll(filepath.Dir(flarePath))

	files := readZip(t, flarePath)

	// There is nothing to explain when the collector answered
	assert.NotContains(t, files, "otel-agent.log")

	// The log of the collector is collected whether or not it answered
	assert.Equal(t, "log of the running collector", string(files["logs/otel-agent.log"]))

	// The effective configuration of the running collector is used, not the configuration files
	assert.Contains(t, string(files["build-info.txt"]), "Extension Version: running-collector")
	assert.Contains(t, string(files["config/runtime-config.yaml"]), "zpages/dd-autoconfigured")

	// Debug sources are fetched, including the ones added by the converter
	assert.Equal(t, profile, files["otel-flare/pprof_debug_pprof_heap.dat"], "binary profiles must be kept as is")
	assert.Contains(t, string(files["otel-flare/zpages/dd-autoconfigured_debug_servicez.dat"]), "<html>")
	assert.Contains(t, string(files["otel-flare/zpages/dd-autoconfigured_debug_missing.err"]), "404")

	// Only the environment variables that are safe to share are kept: a secret can have any name
	assert.Contains(t, string(files["environment.json"]), "DD_SITE")

	for name, content := range files {
		if name == "otel-flare/pprof_debug_pprof_heap.dat" {
			continue
		}
		assert.NotContains(t, string(content), testAPIKey, "API key not scrubbed from %s", name)
		assert.NotContains(t, string(content), testSessionCookie, "session cookie kept in %s", name)
	}
}

// TestCreateOTelFlareCollectsLogs checks that the flare contains the log file of the collector and
// the files its rotation keeps next to it, and no other file of the directory.
func TestCreateOTelFlareCollectsLogs(t *testing.T) {
	globalParams := newGlobalParamsTest(t)
	cfg, ipcComp := newUnreachableCollector(t)

	logDir := t.TempDir()
	cfg.SetInTest("log_file", filepath.Join(logDir, "otel-agent.log"))
	for name, content := range map[string]string{
		"otel-agent.log":     "current log, api_key: " + testAPIKey,
		"otel-agent.log.1":   "rolled log",
		"agent.log":          "log of another component",
		"otel-agent.log.bak": "not a file of the rotation",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(logDir, name), []byte(content), 0644))
	}

	flarePath, err := createOTelFlare(globalParams, cfg, ipcComp.GetClient())
	require.NoError(t, err)
	defer os.RemoveAll(filepath.Dir(flarePath))

	files := readZip(t, flarePath)

	assert.Contains(t, string(files["logs/otel-agent.log"]), "current log")
	assert.Equal(t, "rolled log", string(files["logs/otel-agent.log.1"]))
	assert.NotContains(t, files, "logs/agent.log", "the logs of the other components are not the collector's")
	assert.NotContains(t, files, "logs/otel-agent.log.bak")
	assert.NotContains(t, string(files["logs/otel-agent.log"]), testAPIKey, "API key not scrubbed from the log")
}

// captureColorOutput returns what the command prints on the console
func captureColorOutput(t *testing.T) *bytes.Buffer {
	var output bytes.Buffer
	previousOutput := color.Output
	color.Output = &output
	t.Cleanup(func() { color.Output = previousOutput })
	return &output
}

func TestCollectorLogFile(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		assert.Equal(t, defaultpaths.GetDefaultOTelAgentLogFile(), collectorLogFile(configmock.New(t)))
	})
	t.Run("configured", func(t *testing.T) {
		cfg := configmock.New(t)
		cfg.SetInTest("log_file", "/custom/otel.log")
		assert.Equal(t, "/custom/otel.log", collectorLogFile(cfg))
	})
	t.Run("file logging disabled", func(t *testing.T) {
		cfg := configmock.New(t)
		cfg.SetInTest("disable_file_logging", true)
		assert.Empty(t, collectorLogFile(cfg))
	})
}

func TestIsLogFileOrRoll(t *testing.T) {
	for fileName, expected := range map[string]bool{
		"otel-agent.log":      true,
		"otel-agent.log.1":    true,
		"otel-agent.log.12":   true,
		"otel-agent.log.":     false,
		"otel-agent.log.bak":  false,
		"otel-agent.log.1.gz": false,
		"otel-agent.logs":     false,
		"agent.log":           false,
		"my-otel-agent.log":   false,
	} {
		assert.Equal(t, expected, isLogFileOrRoll(fileName, "otel-agent.log"), fileName)
	}
}

// TestCollectLogFilesWarnings checks that a flare without the log of the collector says why.
func TestCollectLogFilesWarnings(t *testing.T) {
	emptyDir := t.TempDir()
	tests := []struct {
		name    string
		setup   func(cfg config.Component)
		warning string
	}{
		{
			name:    "file logging disabled",
			setup:   func(cfg config.Component) { cfg.SetInTest("disable_file_logging", true) },
			warning: "disable_file_logging",
		},
		{
			name:    "no log file in the directory",
			setup:   func(cfg config.Component) { cfg.SetInTest("log_file", filepath.Join(emptyDir, "otel-agent.log")) },
			warning: "no log file of the collector found at",
		},
		{
			name: "no log directory",
			setup: func(cfg config.Component) {
				cfg.SetInTest("log_file", filepath.Join(emptyDir, "missing", "otel-agent.log"))
			},
			warning: "no log file of the collector found at",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)
			tt.setup(cfg)
			output := captureColorOutput(t)

			assert.Empty(t, collectLogFiles(cfg))
			assert.Contains(t, output.String(), tt.warning)
		})
	}
}

// TestCollectLogFilesSkipsWhatIsNotAFile checks that an entry that looks like a rolled log but isn't
// a regular file is left out without a warning, since it can't be read (a pipe would block).
func TestCollectLogFilesSkipsWhatIsNotAFile(t *testing.T) {
	logDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(logDir, "otel-agent.log"), []byte("log"), 0644))
	require.NoError(t, os.Mkdir(filepath.Join(logDir, "otel-agent.log.1"), 0755))
	cfg := configmock.New(t)
	cfg.SetInTest("log_file", filepath.Join(logDir, "otel-agent.log"))
	output := captureColorOutput(t)

	files := collectLogFiles(cfg)

	require.Len(t, files, 1)
	assert.Equal(t, "logs/otel-agent.log", files[0].name)
	assert.NotContains(t, output.String(), "could not read")
}

func TestFilterEnvironment(t *testing.T) {
	cfg := configmock.New(t)

	env := map[string]string{
		// Bound to a setting of the configuration
		"DD_SITE":     "datadoghq.eu",
		"DD_HOSTNAME": "my-host",
		// Not bound to a setting, but kept
		"HTTPS_PROXY": "http://proxy:3128",
		"https_proxy": "http://proxy:3128",
		"GOMEMLIMIT":  "400MiB",
		// Bound to a sensitive setting of the schema, without _KEY nor _AUTH_TOKEN in the name
		"DD_APM_ADDITIONAL_ENDPOINTS": `{"https://example.com": ["` + testAPIKey + `"]}`,
		// Credentials by their name
		"DD_API_KEY":                  testAPIKey,
		"DD_CLUSTER_AGENT_AUTH_TOKEN": "token",
		// Unknown: it could hold anything
		"SESSION_COOKIE": testSessionCookie,
		"MY_DB_PASSWORD": "hunter2",
		"PATH":           "/usr/bin",
	}

	assert.Equal(t, map[string]string{
		"DD_SITE":     "datadoghq.eu",
		"DD_HOSTNAME": "my-host",
		"HTTPS_PROXY": "http://proxy:3128",
		"https_proxy": "http://proxy:3128",
		"GOMEMLIMIT":  "400MiB",
	}, filterEnvironment(env, cfg))
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

// TestLoadCustomerConfigEnvReference checks that the references the collector expands from the
// environment don't make the customer configuration disappear, with or without the env: scheme.
func TestLoadCustomerConfigEnvReference(t *testing.T) {
	t.Setenv("TEST_FLARE_API_KEY", testAPIKey)

	tests := []struct {
		name      string
		reference string
	}{
		{name: "with the env scheme", reference: "${env:TEST_FLARE_API_KEY}"},
		{name: "without the scheme", reference: "${TEST_FLARE_API_KEY}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "otel-config.yaml")
			configContent := "exporters:\n  datadog:\n    api:\n      key: " + tt.reference + "\n"
			require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

			customerConfig, err := loadCustomerConfig(context.Background(), []string{"file:" + configPath})
			require.NoError(t, err)
			assert.Contains(t, customerConfig, testAPIKey, "the reference should be resolved from the environment")
		})
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

// newSecretBackend writes a secret backend command that runs the given script, and returns its path.
func newSecretBackend(t *testing.T, script string) string {
	if runtime.GOOS == "windows" {
		t.Skip("shell secret backend script not applicable on Windows")
	}
	backend := filepath.Join(t.TempDir(), "secret_backend.sh")
	require.NoError(t, os.WriteFile(backend, []byte(script), 0o700)) //nolint:gosec
	return backend
}

// flareConfig builds the configuration of the flare command like it runs
func flareConfig(t *testing.T, params *subcommands.GlobalParams) config.Component {
	// the flare rebuilds the global config: the mock restores it after the test
	configmock.New(t)

	var cfg config.Component
	fxutil.TestOneShotSubcommand(t,
		[]*cobra.Command{MakeCommand(func() *subcommands.GlobalParams { return params })},
		[]string{"flare", "--send"},
		makeFlare,
		func(c config.Component) { cfg = c },
	)
	return cfg
}

// flareAPIKey returns the API key the flare would be uploaded with
func flareAPIKey(t *testing.T, params *subcommands.GlobalParams) string {
	return flareConfig(t, params).GetString("api_key")
}

// TestFlareCommandResolvesSecrets checks that the flare resolves the ENC[] handles of the
// configuration, like the core Agent flare, instead of uploading with the handle as API key.
func TestFlareCommandResolvesSecrets(t *testing.T) {
	backend := newSecretBackend(t, "#!/bin/sh\necho '{\"flare_api_key\": {\"value\": \""+testAPIKey+"\"}}'\n")

	t.Run("configuration file", func(t *testing.T) {
		coreConfig := filepath.Join(t.TempDir(), "datadog.yaml")
		content := "api_key: ENC[flare_api_key]\nsecret_backend_command: " + backend + "\n"
		require.NoError(t, os.WriteFile(coreConfig, []byte(content), 0o600))

		assert.Equal(t, testAPIKey, flareAPIKey(t, &subcommands.GlobalParams{CoreConfPath: coreConfig}))
	})

	// loading the configuration stops before it resolves the secrets when it finds no file
	t.Run("environment only", func(t *testing.T) {
		t.Setenv("DD_API_KEY", "ENC[flare_api_key]")
		t.Setenv("DD_SECRET_BACKEND_COMMAND", backend)

		assert.Equal(t, testAPIKey, flareAPIKey(t, &subcommands.GlobalParams{ConfigName: "datadog-otel"}))
	})
}

// TestFlareCommandSecretBackendFailure checks that a failing secret backend doesn't prevent the
// flare, which is run to diagnose such problems, and that the flare tells it.
func TestFlareCommandSecretBackendFailure(t *testing.T) {
	t.Setenv("DD_API_KEY", "ENC[flare_api_key]")
	t.Setenv("DD_SECRET_BACKEND_COMMAND", newSecretBackend(t, "#!/bin/sh\nexit 1\n"))
	output := captureColorOutput(t)

	assert.Equal(t, "ENC[flare_api_key]", flareAPIKey(t, &subcommands.GlobalParams{ConfigName: "datadog-otel"}))
	assert.Contains(t, output.String(), "could not resolve the secrets")
}

// TestMain keeps the tests off the configuration file of an Agent installed on the machine
func TestMain(m *testing.M) {
	defaultConfFile = func() string { return "" }
	os.Exit(m.Run())
}

// useDefaultConfFile makes the flare command look for its default datadog.yaml at path
func useDefaultConfFile(t *testing.T, path string) {
	previous := defaultConfFile
	defaultConfFile = func() string { return path }
	t.Cleanup(func() { defaultConfFile = previous })
}

// TestFlareCommandAppliesEnvironmentSettings checks that the flare applies the settings that the
// environment gives when there is no configuration file, like `otel-agent run` does.
func TestFlareCommandAppliesEnvironmentSettings(t *testing.T) {
	t.Run("proxy", func(t *testing.T) {
		t.Setenv("DD_PROXY_HTTPS", "http://proxy.example.com:3128")

		cfg := flareConfig(t, &subcommands.GlobalParams{ConfigName: "datadog-otel"})

		assert.Equal(t, "http://proxy.example.com:3128", cfg.GetString("proxy.https"))
	})

	t.Run("scrubber keys", func(t *testing.T) {
		t.Setenv("DD_SCRUBBER_ADDITIONAL_KEYS", "flare-test-additional-key")

		flareConfig(t, &subcommands.GlobalParams{ConfigName: "datadog-otel"})

		scrubbed, err := scrubber.ScrubYamlString("flare-test-additional-key: some-value\n")
		require.NoError(t, err)
		assert.NotContains(t, scrubbed, "some-value")
	})
}

// TestFlareCommandResolvesSecretProxy checks that the proxy can be given by a secret handle, which is
// why the proxy is loaded before the secrets are resolved.
func TestFlareCommandResolvesSecretProxy(t *testing.T) {
	backend := newSecretBackend(t, "#!/bin/sh\necho '{\"flare_proxy\": {\"value\": \"http://proxy.example.com:3128\"}}'\n")
	t.Setenv("DD_PROXY_HTTPS", "ENC[flare_proxy]")
	t.Setenv("DD_SECRET_BACKEND_COMMAND", backend)

	cfg := flareConfig(t, &subcommands.GlobalParams{ConfigName: "datadog-otel"})

	assert.Equal(t, "http://proxy.example.com:3128", cfg.GetString("proxy.https"))
}

// TestFlareCommandDefaultCoreConfig checks that, like `otel-agent run`, the flare loads the default
// datadog.yaml of the Agent when --core-config isn't given, unless the collector is standalone.
func TestFlareCommandDefaultCoreConfig(t *testing.T) {
	newConfFile := func(apiKey string) string {
		path := filepath.Join(t.TempDir(), "datadog.yaml")
		require.NoError(t, os.WriteFile(path, []byte("api_key: "+apiKey+"\n"), 0o600))
		return path
	}
	useDefaultConfFile(t, newConfFile(testAPIKey))

	t.Run("bundled", func(t *testing.T) {
		t.Setenv("DD_OTEL_STANDALONE", "false")

		assert.Equal(t, testAPIKey, flareAPIKey(t, &subcommands.GlobalParams{}))
	})

	t.Run("standalone", func(t *testing.T) {
		t.Setenv("DD_OTEL_STANDALONE", "true")

		assert.NotEqual(t, testAPIKey, flareAPIKey(t, &subcommands.GlobalParams{}), "the default datadog.yaml was loaded")
	})

	t.Run("explicit path", func(t *testing.T) {
		t.Setenv("DD_OTEL_STANDALONE", "false")

		assert.Equal(t, "another-key", flareAPIKey(t, &subcommands.GlobalParams{CoreConfPath: newConfFile("another-key")}))
	})
}

// TestFlareCommandIgnoresBrokenCoreConfig checks that a configuration file that can't be loaded
// doesn't prevent the flare, which is run to diagnose such problems, like the core Agent flare.
func TestFlareCommandIgnoresBrokenCoreConfig(t *testing.T) {
	broken := filepath.Join(t.TempDir(), "datadog.yaml")
	require.NoError(t, os.WriteFile(broken, []byte("api_key: [unterminated\n  foo: : :\n"), 0o600))

	assert.NotPanics(t, func() { flareConfig(t, &subcommands.GlobalParams{CoreConfPath: broken}) })
}
