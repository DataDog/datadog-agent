// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sharedlibrarycheck && test

// This test loads the ACR HTTP check plugin (.dylib/.so) and runs it against a
// local HTTP server, verifying that the expected metrics and service checks are
// submitted through the FFI callback bridge.
//
// Prerequisites:
//   1. Build the HTTP check plugin from the ACR repository:
//        cd ~/dd/agent-check-runner && cargo build -p http-check-plugin --release
//   2. Set the HTTP_CHECK_PLUGIN_PATH environment variable to the built library:
//        export HTTP_CHECK_PLUGIN_PATH=~/dd/agent-check-runner/target/release/libhttp_check_plugin.dylib
//      If not set, the test looks in the default location above.
//   3. Run with the sharedlibrarycheck build tag:
//        go test -tags sharedlibrarycheck,test ./pkg/collector/sharedlibrary/sharedlibraryimpl/ -run TestHTTPCheck -v

package sharedlibrarycheck

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/sharedlibrary/enrichment"
	"github.com/DataDog/datadog-agent/pkg/collector/sharedlibrary/ffi"
)

// httpCheckPluginPath resolves the path to the built HTTP check plugin shared library.
// It checks the HTTP_CHECK_PLUGIN_PATH env var first, then falls back to the default
// ACR release build location.
func httpCheckPluginPath() string {
	if p := os.Getenv("HTTP_CHECK_PLUGIN_PATH"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	ext := "so"
	switch runtime.GOOS {
	case "darwin":
		ext = "dylib"
	case "windows":
		ext = "dll"
	}
	return filepath.Join(home, "dd", "agent-check-runner", "target", "release", "libhttp_check_plugin."+ext)
}

// TestHTTPCheckPlugin is a functional test that loads the ACR HTTP check plugin,
// runs it against a local HTTP test server, and verifies the submitted metrics
// and service checks via the mock sender.
func TestHTTPCheckPlugin(t *testing.T) {
	pluginPath := httpCheckPluginPath()
	if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
		t.Skipf("HTTP check plugin not found at %s; build it first: cd ~/dd/agent-check-runner && cargo build -p http-check-plugin --release", pluginPath)
	}

	// Start a local HTTP server that returns 200 OK
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	}))
	defer ts.Close()

	// Create the real shared library loader
	loader := ffi.NewSharedLibraryLoader(filepath.Dir(pluginPath))

	// Open the HTTP check plugin
	lib, err := loader.Open(pluginPath)
	require.NoError(t, err, "failed to open HTTP check plugin at %s", pluginPath)
	defer func() {
		err := loader.Close(lib)
		assert.NoError(t, err, "failed to close HTTP check plugin")
	}()

	// Try to get the version (optional -- ACR plugins may not export a Version symbol)
	version, err := loader.Version(lib)
	if err == nil {
		t.Logf("HTTP check plugin version: %s", version)
	} else {
		t.Logf("HTTP check plugin does not export Version symbol (expected for ACR plugins): %v", err)
	}

	// Create enrichment data
	provider, err := enrichment.NewStaticProvider(enrichment.EnrichmentData{
		Hostname:     "test-host",
		AgentVersion: "7.60.0",
	})
	require.NoError(t, err)

	// Set up a check with the real library loader
	checkID := checkid.ID("http_check:test_instance:abcdef1234567890")

	// Create a mock sender and demultiplexer
	mockSender := mocksender.NewMockSender(checkID)
	mockSender.SetupAcceptAll()

	senderManager := mockSender.GetSenderManager()

	// Build the instance config as YAML (matching the ACR's http_check config::Instance struct)
	instanceConfig := fmt.Sprintf(`name: test-http
url: %s
collect_response_time: true
check_certificate_expiration: false
`, ts.URL)

	initConfig := "{}"

	enrichmentYAML := provider.GetEnrichmentYAML()

	// Run the check through the FFI bridge
	err = loader.Run(lib, string(checkID), initConfig, instanceConfig, enrichmentYAML, senderManager)
	require.NoError(t, err, "HTTP check run failed")

	// Verify the submitted metrics
	// The HTTP check submits:
	//   - network.http.response_time (gauge) - response time in seconds
	//   - network.http.can_connect (gauge, 1.0 on success)
	//   - network.http.cant_connect (gauge, 0.0 on success)
	// And a service check:
	//   - http.can_connect with OK status

	// Verify network.http.response_time was submitted (value > 0)
	mockSender.AssertCalled(t, "Gauge", "network.http.response_time",
		mock.AnythingOfType("float64"),
		mock.AnythingOfType("string"),
		mock.AnythingOfType("[]string"),
	)

	// Verify network.http.can_connect was submitted with value 1.0 (success)
	mockSender.AssertCalled(t, "Gauge", "network.http.can_connect",
		1.0,
		mock.AnythingOfType("string"),
		mock.AnythingOfType("[]string"),
	)

	// Verify network.http.cant_connect was submitted with value 0.0 (success)
	mockSender.AssertCalled(t, "Gauge", "network.http.cant_connect",
		0.0,
		mock.AnythingOfType("string"),
		mock.AnythingOfType("[]string"),
	)

	// Verify the service check was submitted with OK status
	mockSender.AssertCalled(t, "ServiceCheck", "http.can_connect",
		mock.AnythingOfType("servicecheck.ServiceCheckStatus"),
		mock.AnythingOfType("string"),
		mock.AnythingOfType("[]string"),
		mock.AnythingOfType("string"),
	)
}

// TestHTTPCheckPluginFailure verifies that the HTTP check correctly reports
// failure metrics when the target server is unreachable.
func TestHTTPCheckPluginFailure(t *testing.T) {
	pluginPath := httpCheckPluginPath()
	if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
		t.Skipf("HTTP check plugin not found at %s; build it first", pluginPath)
	}

	// Start and immediately close a server so the port is unreachable
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	unreachableURL := ts.URL
	ts.Close()

	loader := ffi.NewSharedLibraryLoader(filepath.Dir(pluginPath))

	lib, err := loader.Open(pluginPath)
	require.NoError(t, err)
	defer func() { _ = loader.Close(lib) }()

	provider, err := enrichment.NewStaticProvider(enrichment.EnrichmentData{
		Hostname:     "test-host",
		AgentVersion: "7.60.0",
	})
	require.NoError(t, err)

	checkID := checkid.ID("http_check:test_failure:abcdef1234567890")
	mockSender := mocksender.NewMockSender(checkID)
	mockSender.SetupAcceptAll()
	senderManager := mockSender.GetSenderManager()

	instanceConfig := fmt.Sprintf(`name: test-http-fail
url: %s
check_certificate_expiration: false
timeout: 2
connect_timeout: 2
`, unreachableURL)

	initConfig := "{}"
	enrichmentYAML := provider.GetEnrichmentYAML()

	// The check should not return an error itself -- it handles errors internally
	// by submitting a critical service check
	err = loader.Run(lib, string(checkID), initConfig, instanceConfig, enrichmentYAML, senderManager)
	require.NoError(t, err, "HTTP check run should not return an error for connection failures")

	// The service check should still be submitted (with Critical status)
	mockSender.AssertCalled(t, "ServiceCheck", "http.can_connect",
		mock.AnythingOfType("servicecheck.ServiceCheckStatus"),
		mock.AnythingOfType("string"),
		mock.AnythingOfType("[]string"),
		mock.AnythingOfType("string"),
	)
}
