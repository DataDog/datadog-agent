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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
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
		// Kept by name: the proxy, the path of the auth token, which the _AUTH_TOKEN rule would drop, and the pod
		"DD_PROXY_HTTP":           "http://proxy:3128",
		"DD_PROXY_HTTPS":          "http://proxy:3128",
		"DD_PROXY_NO_PROXY":       "localhost",
		"DD_AUTH_TOKEN_FILE_PATH": "/etc/datadog-agent/auth_token",
		"HOSTNAME":                "my-pod",
		// Only those names: a longer one with _AUTH_TOKEN is dropped
		"MY_AUTH_TOKEN_FILE_PATH": "/etc/token",
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
		"DD_SITE":                 "datadoghq.eu",
		"DD_HOSTNAME":             "my-host",
		"HTTPS_PROXY":             "http://proxy:3128",
		"https_proxy":             "http://proxy:3128",
		"GOMEMLIMIT":              "400MiB",
		"DD_PROXY_HTTP":           "http://proxy:3128",
		"DD_PROXY_HTTPS":          "http://proxy:3128",
		"DD_PROXY_NO_PROXY":       "localhost",
		"DD_AUTH_TOKEN_FILE_PATH": "/etc/datadog-agent/auth_token",
		"HOSTNAME":                "my-pod",
	}, filterEnvironment(env, cfg))
}

// TestCreateOTelFlareEnvironmentProxy checks that the proxy of the environment is in the flare, without the
// password of its URL.
func TestCreateOTelFlareEnvironmentProxy(t *testing.T) {
	const password = "hunter2-proxy-password"
	t.Setenv("DD_PROXY_HTTPS", "http://user:"+password+"@proxy.example.com:3128")
	globalParams := newGlobalParamsTest(t)
	cfg, ipcComp := newUnreachableCollector(t)

	flarePath, err := createOTelFlare(globalParams, cfg, ipcComp.GetClient())
	require.NoError(t, err)
	defer os.RemoveAll(filepath.Dir(flarePath))
	files := readZip(t, flarePath)

	for name, content := range files {
		assert.NotContains(t, string(content), password, "the password of the proxy in %s", name)
	}
	var environment map[string]string
	require.NoError(t, json.Unmarshal(files["environment.json"], &environment))
	assert.Contains(t, environment["DD_PROXY_HTTPS"], "proxy.example.com:3128")
}

// TestCreateOTelFlareJSONEnvironmentValue checks the credentials in a variable whose value is JSON, like the
// configuration of the secret backend.
func TestCreateOTelFlareJSONEnvironmentValue(t *testing.T) {
	const secret = "sid-0123456789"
	t.Setenv("DD_SECRET_BACKEND_CONFIG", `{"vault_session":{"vault_address":"https://vault.example.com","vault_secret_id":"`+secret+`"}}`)
	globalParams := newGlobalParamsTest(t)
	cfg, ipcComp := newUnreachableCollector(t)

	flarePath, err := createOTelFlare(globalParams, cfg, ipcComp.GetClient())
	require.NoError(t, err)
	defer os.RemoveAll(filepath.Dir(flarePath))

	for name, content := range readZip(t, flarePath) {
		assert.NotContains(t, string(content), secret, "the secret of the backend in %s", name)
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

// TestScrubResponseJSONEnvironmentValue checks that the value of a variable that is JSON is scrubbed as JSON, and that
// the rest of it is kept.
func TestScrubResponseJSONEnvironmentValue(t *testing.T) {
	data := &extensiontypes.Response{
		Environment: map[string]string{
			"JSON_OBJECT": `{"vault_address":"https://vault.example.com","vault_secret_id":"sid-0123456789","retries":3}`,
			"JSON_ARRAY":  ` [{"name":"a","password":"hunter2"}]`,
			"PLAIN":       "/usr/bin",
		},
	}

	scrubbed, err := scrubResponse(data)
	require.NoError(t, err)

	assert.JSONEq(t, `{"vault_address":"https://vault.example.com","vault_secret_id":"********","retries":3}`, scrubbed.Environment["JSON_OBJECT"])
	assert.JSONEq(t, `[{"name":"a","password":"********"}]`, scrubbed.Environment["JSON_ARRAY"])
	assert.Equal(t, "/usr/bin", scrubbed.Environment["PLAIN"])
	// the input is not modified
	assert.Contains(t, data.Environment["JSON_OBJECT"], "sid-0123456789")
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

// TestCreateOTelFlareKeepsEnvironmentValuesOut checks that the flare built from the configuration
// files, which can't tell which settings are secrets, has the environment references of the
// configurations as they are, like the configuration of the live flare, and not the values of the
// variables, that can be secrets with a name that no scrubber recognizes.
func TestCreateOTelFlareKeepsEnvironmentValuesOut(t *testing.T) {
	const secret = "hc-secret-value-0123456789"
	t.Setenv("FLARE_TEST_HEADER", secret)
	t.Setenv("FLARE_TEST_OTHER_HEADER", secret) // the same value: the names must not be mixed up
	t.Setenv("FLARE_TEST_ORIGIN", secret)
	t.Setenv("FLARE_TEST_ATTRIBUTE", secret)
	t.Setenv("TEST_FLARE_API_KEY", testAPIKey)
	configPath := filepath.Join(t.TempDir(), "otel-config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`receivers:
  otlp:
    protocols:
      grpc: {}
      http:
        cors:
          allowed_origins:
            - ${env:FLARE_TEST_ORIGIN}
processors:
  attributes:
    actions:
      - key: team
        action: upsert
        value: ${env:FLARE_TEST_ATTRIBUTE}
exporters:
  otlp:
    endpoint: example.com:4317
    headers:
      x-honeycomb-team: ${env:FLARE_TEST_HEADER}
      x-other-team: ${FLARE_TEST_OTHER_HEADER}
  datadog:
    api:
      key: ${env:TEST_FLARE_API_KEY}
service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [attributes]
      exporters: [otlp, datadog]
`), 0644))
	cfg, ipcComp := newUnreachableCollector(t)

	flarePath, err := createOTelFlare(&subcommands.GlobalParams{ConfPaths: []string{"file:" + configPath}}, cfg, ipcComp.GetClient())
	require.NoError(t, err)
	defer os.RemoveAll(filepath.Dir(flarePath))
	files := readZip(t, flarePath)

	for name, content := range files {
		assert.NotContains(t, string(content), secret, "value of the environment in %s", name)
	}
	for _, name := range []string{"config/customer-config.yaml", "config/runtime-config.yaml", "config/env-config.yaml"} {
		assert.Contains(t, string(files[name]), "x-honeycomb-team: ${env:FLARE_TEST_HEADER}", name)
		assert.Contains(t, string(files[name]), "x-other-team: ${env:FLARE_TEST_OTHER_HEADER}", name)
		// in a list of strings, and in a list of maps
		assert.Contains(t, string(files[name]), "- ${env:FLARE_TEST_ORIGIN}", name)
		assert.Contains(t, string(files[name]), "value: ${env:FLARE_TEST_ATTRIBUTE}", name)
	}
	// like the env configuration of the live flare, only the settings that come from the environment
	envConfig := string(files["config/env-config.yaml"])
	assert.NotContains(t, envConfig, "example.com", "env-config.yaml has a setting that doesn't come from the environment")
	assert.NotContains(t, envConfig, "upsert", "env-config.yaml has the other settings of a list item")
}

// TestCreateOTelFlareEnvLocation checks that a collector configuration given as an env: location,
// like the standalone collector of ECS Fargate does, is still loaded from the configuration files,
// next to another one, and that the references inside it stay references.
func TestCreateOTelFlareEnvLocation(t *testing.T) {
	const secret = "hc-secret-value-0123456789"
	t.Setenv("FLARE_TEST_OTHER_CONFIG", "receivers:\n  otlp:\n    protocols:\n      grpc: {}\n")
	t.Setenv("FLARE_TEST_HEADER", secret)
	t.Setenv("FLARE_TEST_CONFIG", `exporters:
  otlp:
    endpoint: example.com:4317
    headers:
      x-honeycomb-team: ${env:FLARE_TEST_HEADER}
service:
  pipelines:
    traces:
      exporters: [otlp]
`)
	cfg, ipcComp := newUnreachableCollector(t)

	flarePath, err := createOTelFlare(&subcommands.GlobalParams{ConfPaths: []string{"env:FLARE_TEST_CONFIG", "env:FLARE_TEST_OTHER_CONFIG"}}, cfg, ipcComp.GetClient())
	require.NoError(t, err)
	defer os.RemoveAll(filepath.Dir(flarePath))
	files := readZip(t, flarePath)

	for name, content := range files {
		assert.NotContains(t, string(content), secret, "value of the environment in %s", name)
	}
	for _, name := range []string{"config/customer-config.yaml", "config/runtime-config.yaml"} {
		require.Contains(t, files, name)
		assert.Contains(t, string(files[name]), "x-honeycomb-team: ${env:FLARE_TEST_HEADER}", name)
		assert.Contains(t, string(files[name]), "receivers:", "the other location is missing in %s", name)
	}
	require.Contains(t, files, "config/env-config.yaml")
	assert.Contains(t, string(files["config/env-config.yaml"]), "x-honeycomb-team: ${env:FLARE_TEST_HEADER}")
}

// TestCreateOTelFlareKeepsJSONFilesValid checks that the JSON files of the archive stay valid, with the whole
// configuration, when the configuration has a key that the scrubber hides. The configuration is one line of
// otel-response.json, and a scrubber that works line by line cuts that line at the key.
func TestCreateOTelFlareKeepsJSONFilesValid(t *testing.T) {
	const password = "pw-open-0123456789"
	configPath := filepath.Join(t.TempDir(), "otel-config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`receivers:
  prometheus:
    config:
      scrape_configs:
        - job_name: scrape
          basic_auth:
            username: scrape-user
            password: `+password+`
exporters:
  debug: {}
service:
  pipelines:
    metrics:
      receivers: [prometheus]
      exporters: [debug]
`), 0644))
	cfg, ipcComp := newUnreachableCollector(t)

	flarePath, err := createOTelFlare(&subcommands.GlobalParams{ConfPaths: []string{"file:" + configPath}}, cfg, ipcComp.GetClient())
	require.NoError(t, err)
	defer os.RemoveAll(filepath.Dir(flarePath))
	files := readZip(t, flarePath)

	for name, content := range files {
		assert.NotContains(t, string(content), password, "the password in %s", name)
		if strings.HasSuffix(name, ".json") {
			assert.True(t, json.Valid(content), "%s is not valid JSON", name)
		}
	}

	// The settings that follow the password are not lost
	var response map[string]any
	require.NoError(t, json.Unmarshal(files["otel-response.json"], &response))
	configurations := 0
	for field, value := range response {
		text, ok := value.(string)
		if !ok || !strings.Contains(text, "password:") {
			continue
		}
		configurations++
		assert.Contains(t, text, "job_name: scrape", "the settings after the password are lost in %s", field)
		assert.Contains(t, text, "service:", "the end of the configuration is lost in %s", field)
	}
	assert.Positive(t, configurations, "no configuration with the password in otel-response.json")
}

// TestCreateFlareArchiveScrubsJSONAsJSON checks that the JSON files of the archive are scrubbed as JSON, and the
// other files as text.
func TestCreateFlareArchiveScrubsJSONAsJSON(t *testing.T) {
	const secret = "hunter2"
	tests := []struct {
		name      string
		file      string
		content   string
		kept      string
		validJSON bool
	}{
		{"key in a string", "response.json", `{"config": "a:\n  password: ` + secret + `\n  b: c\n"}`, "b: c", true},
		{"key of an object", "environment.json", `{"password": "` + secret + `", "b": "c"}`, `"b": "c"`, true},
		{"password of a URL", "environment.json", `{"X": "http://user:` + secret + `@host:3128"}`, "host:3128", true},
		{"JSON that does not parse", "broken.json", `{"a": "password: ` + secret, "", false},
		{"not JSON", "config.yaml", "password: " + secret + "\nb: c\n", "b: c", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), "flare.zip")
			require.NoError(t, createFlareArchive(archive, []flareFile{{name: tt.file, content: []byte(tt.content), scrub: true}}))
			content := readZip(t, archive)[tt.file]

			assert.NotContains(t, string(content), secret)
			assert.Contains(t, string(content), tt.kept)
			assert.Equal(t, tt.validJSON, json.Valid(content), string(content))
		})
	}
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
// environment don't make the customer configuration disappear, with or without the env: scheme,
// and that they stay references: the value of a variable can be a secret that no scrubber recognizes.
func TestLoadCustomerConfigEnvReference(t *testing.T) {
	t.Setenv("TEST_FLARE_API_KEY", testAPIKey)

	tests := []struct {
		name      string
		reference string
		want      string
	}{
		{name: "with the env scheme", reference: "${env:TEST_FLARE_API_KEY}", want: "${env:TEST_FLARE_API_KEY}"},
		{name: "without the scheme", reference: "${TEST_FLARE_API_KEY}", want: "${env:TEST_FLARE_API_KEY}"},
		{name: "with a default", reference: "${env:TEST_FLARE_API_KEY:-fallback-value}", want: "${env:TEST_FLARE_API_KEY:-fallback-value}"},
		{name: "in a string", reference: "prefix-${env:TEST_FLARE_API_KEY}-suffix", want: "prefix-${env:TEST_FLARE_API_KEY}-suffix"},
		{name: "unset variable", reference: "${env:UNSET_FLARE_TEST_VARIABLE}", want: "${env:UNSET_FLARE_TEST_VARIABLE}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "otel-config.yaml")
			configContent := "exporters:\n  datadog:\n    api:\n      key: " + tt.reference + "\n"
			require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

			customerConfig, err := loadCustomerConfig(context.Background(), []string{"file:" + configPath})
			require.NoError(t, err)
			assert.Contains(t, customerConfig, "key: "+tt.want, "the reference should stay a reference, like in the live flare")
			assert.NotContains(t, customerConfig, testAPIKey, "the value of the environment should not be in the configuration")
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

// TestCollectDebugSourcesBoundsTheSize checks that a response larger than the limit is not kept and not read to the
// end, and that the sources stop being kept once the size budget of all of them is used.
func TestCollectDebugSourcesBoundsTheSize(t *testing.T) {
	previousOne, previousTotal := maxDebugSourceBytes, maxDebugSourcesTotalBytes
	maxDebugSourceBytes, maxDebugSourcesTotalBytes = 4<<10, 6<<10
	t.Cleanup(func() { maxDebugSourceBytes, maxDebugSourcesTotalBytes = previousOne, previousTotal })

	const large = 64 << 20
	var sentOfTheLarge atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a":
			_, _ = w.Write(bytes.Repeat([]byte("a"), 2<<10))
		case "/b":
			_, _ = w.Write(bytes.Repeat([]byte("b"), 3<<10))
		case "/c":
			_, _ = w.Write(bytes.Repeat([]byte("c"), 2<<10))
		case "/large":
			chunk := bytes.Repeat([]byte("x"), 64<<10)
			for sentOfTheLarge.Load() < large {
				n, err := w.Write(chunk)
				sentOfTheLarge.Add(int64(n))
				if err != nil {
					return
				}
			}
		}
	}))
	defer server.Close()
	captureColorOutput(t)

	files := collectDebugSources(map[string]extensiontypes.OTelFlareSource{
		"s": {URLs: []string{server.URL + "/a", server.URL + "/large", server.URL + "/b", server.URL + "/c"}},
	})
	content := map[string]string{}
	for _, f := range files {
		content[f.name] = string(f.content)
	}

	assert.Equal(t, strings.Repeat("a", 2<<10), content["otel-flare/s_a.dat"])
	assert.Contains(t, content["otel-flare/s_large.err"], "larger than", "a response over the limit is an error entry")
	assert.NotContains(t, content, "otel-flare/s_large.dat")
	assert.Equal(t, strings.Repeat("b", 3<<10), content["otel-flare/s_b.dat"], "4 KiB of the budget are left, 3 KiB fit")
	assert.Contains(t, content["otel-flare/s_c.err"], "budget", "1 KiB of the budget is left, 2 KiB don't fit")
	assert.Less(t, sentOfTheLarge.Load(), int64(large), "the response is read to the end")
}

// TestCollectLogFilesKeepsABoundedTail checks that only the end of each log file is kept, from the start of a line,
// that the newest files come first, and that the files that don't fit in the total size are left out.
func TestCollectLogFilesKeepsABoundedTail(t *testing.T) {
	previousFile, previousTotal := maxLogFileBytes, maxLogTotalBytes
	maxLogFileBytes, maxLogTotalBytes = 1<<10, 2<<10
	t.Cleanup(func() { maxLogFileBytes, maxLogTotalBytes = previousFile, previousTotal })

	dir := t.TempDir()
	for _, name := range []string{"otel-agent.log", "otel-agent.log.1", "otel-agent.log.2", "otel-agent.log.10"} {
		var content strings.Builder
		for i := 1; i <= 100; i++ {
			fmt.Fprintf(&content, "%s line %03d\n", name, i)
		}
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content.String()), 0644))
	}
	cfg := configmock.New(t)
	cfg.SetInTest("log_file", filepath.Join(dir, "otel-agent.log"))
	output := captureColorOutput(t)

	files := collectLogFiles(cfg)

	var names []string
	for _, f := range files {
		names = append(names, f.name)
	}
	assert.Equal(t, []string{"logs/otel-agent.log", "logs/otel-agent.log.1"}, names, "the newest files, in the order of their rotation")
	for _, f := range files {
		lines := strings.Split(strings.TrimSuffix(string(f.content), "\n"), "\n")
		assert.Contains(t, lines[0], "the last", "the first line says that the beginning is left out")
		assert.True(t, strings.HasPrefix(lines[1], strings.TrimPrefix(f.name, "logs/")+" line "), "the tail starts at the start of a line: %q", lines[1])
		assert.Equal(t, strings.TrimPrefix(f.name, "logs/")+" line 100", lines[len(lines)-1], "the tail is the end of the file")
		assert.LessOrEqual(t, int64(len(f.content)), maxLogFileBytes+200, f.name)
	}
	assert.Contains(t, output.String(), "otel-agent.log.2", "the warning says where the files are left out")
}

// failingWriter fails every write, like a disk that is full
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("no space left on device")
}

// TestWriteFlareArchiveReportsWriteErrors checks that a failure to write the end of the archive, which the zip writer
// keeps in a buffer until it is closed, is reported and not lost.
func TestWriteFlareArchiveReportsWriteErrors(t *testing.T) {
	// Small enough for nothing to reach the writer before the archive is finished
	err := writeFlareArchive(failingWriter{}, []flareFile{{name: "a.txt", content: []byte("a"), scrub: true}})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no space left on device")
}

// TestCreateArchiveInTempDirRemovesTheDirOnFailure checks that the temporary directory of a flare whose archive
// can't be created is not left behind.
func TestCreateArchiveInTempDirRemovesTheDirOnFailure(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)

	_, err := createArchiveInTempDir(nil, func(string, []flareFile) error { return errors.New("no space left on device") })

	require.ErrorContains(t, err, "no space left on device")
	entries, readErr := os.ReadDir(tmp)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "the temporary directory is left behind")
}

// TestDiscoverDebugSourcesOddEndpoints checks that a debug extension whose endpoint is not a string, which a
// collector configuration can hold, is skipped with a warning, and doesn't crash the flare. The extensions that
// are fine, and the one that has no endpoint and gets the default one, are kept.
func TestDiscoverDebugSourcesOddEndpoints(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "otel-config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`extensions:
  pprof:
    endpoint:
  zpages:
    endpoint: 55679
  health_check:
    endpoint:
      host: localhost
  pprof/list:
    endpoint: [localhost:1777]
  zpages/empty:
    endpoint: ""
  health_check/ok:
    endpoint: localhost:13133
  pprof/default: {}
`), 0644))
	output := captureColorOutput(t)

	var sources map[string]extensiontypes.OTelFlareSource
	require.NotPanics(t, func() {
		sources = discoverDebugSources(context.TODO(), []string{"file:" + configPath})
	})

	assert.Len(t, sources, 2)
	require.Contains(t, sources, "health_check/ok")
	assert.Equal(t, []string{"http://localhost:13133"}, sources["health_check/ok"].URLs)
	require.Contains(t, sources, "pprof/default")
	assert.Equal(t, "http://localhost:1777/debug/pprof/heap", sources["pprof/default"].URLs[0])

	// The extensions that are skipped are named in a warning, the others are not
	for _, skipped := range []string{"pprof", "zpages", "health_check", "pprof/list", "zpages/empty"} {
		assert.Contains(t, output.String(), fmt.Sprintf("Warning: ignoring the debug extension %q:", skipped))
	}
	for _, kept := range []string{"health_check/ok", "pprof/default"} {
		assert.NotContains(t, output.String(), fmt.Sprintf("%q", kept))
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
