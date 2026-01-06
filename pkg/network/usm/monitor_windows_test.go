// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows && npm && test

package usm

import (
	"fmt"
	"io"
	nethttp "net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	iistestutil "github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
	tracetestutil "github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/stretchr/testify/require"
)

// setupWindowsMonitor initializes the Windows driver and creates a USM monitor for testing.
// It skips the test if the driver cannot be initialized (requires admin privileges).
func setupWindowsMonitor(t *testing.T, cfg *config.Config) Monitor {
	t.Helper()

	if err := driver.Init(); err != nil {
		t.Skipf("driver initialization failed (may require admin privileges): %v", err)
	}

	if err := driver.Start(); err != nil {
		t.Skipf("driver start failed: %v", err)
	}

	di, err := network.NewDriverInterface(cfg, driver.NewHandle, nil)
	if err != nil {
		t.Skipf("driver interface creation failed: %v", err)
	}
	require.NotNil(t, di)

	t.Cleanup(func() {
		di.Close()
		driver.Stop()
	})

	dh := di.GetHandle()
	require.NotNil(t, dh)

	monitor, err := NewWindowsMonitor(cfg, dh)
	require.NoError(t, err)
	require.NotNil(t, monitor)

	monitor.Start()
	t.Cleanup(func() {
		monitor.Stop()
	})

	return monitor
}

// getHTTPCfg returns a config with HTTP monitoring enabled.
func getHTTPCfg() *config.Config {
	cfg := config.New()
	cfg.EnableHTTPMonitoring = true
	return cfg
}

// getHTTPLikeProtocolStats retrieves HTTP stats from the monitor for the specified protocol type.
func getHTTPLikeProtocolStats(t *testing.T, monitor Monitor, protocolType protocols.ProtocolType) map[http.Key]*http.RequestStats {
	t.Helper()

	allStats := monitor.GetHTTPStats()
	require.NotNil(t, allStats)

	statsObj, ok := allStats[protocolType]
	if !ok {
		return make(map[http.Key]*http.RequestStats)
	}

	httpStats, ok := statsObj.(map[http.Key]*http.RequestStats)
	require.True(t, ok, "expected map[http.Key]*http.RequestStats, got %T", statsObj)

	return httpStats
}

// verifyHTTPStats checks if HTTP stats matching the criteria exist and optionally validates them with a custom function.
// Returns true if stats are found and pass validation (if provided).
func verifyHTTPStats(t *testing.T, monitor Monitor, serverPort int, testPath string, expectedStatus int, validateFn func(*testing.T, *http.RequestStat) bool) bool {
	t.Helper()

	stats := getHTTPLikeProtocolStats(t, monitor, protocols.HTTP)

	for key, reqStats := range stats {
		t.Logf("Found: %v %s [%d:%d]", key.Method, key.Path.Content.Get(), key.SrcPort, key.DstPort)

		if key.Method != http.MethodGet {
			continue
		}
		if !strings.HasSuffix(key.Path.Content.Get(), testPath) {
			continue
		}
		if key.SrcPort != uint16(serverPort) && key.DstPort != uint16(serverPort) {
			continue
		}

		if stat := reqStats.Data[uint16(expectedStatus)]; stat != nil && stat.Count >= 1 {
			// If no custom validation, return true
			if validateFn == nil {
				return true
			}
			// Run custom validation
			return validateFn(t, stat)
		}
	}

	return false
}

// verifyIISDynamicTags validates that IIS-specific dynamic tags are present in the request stats.
func verifyIISDynamicTags(t *testing.T, stat *http.RequestStat, expectedTags map[string]struct{}) bool {
	t.Helper()

	// Verify IIS dynamic tags are present
	require.NotNil(t, stat.DynamicTags, "Dynamic tags should be present for IIS traffic")

	statsTags := make(map[string]struct{})
	for _, tag := range stat.DynamicTags.GetAll() {
		if name, _, ok := strings.Cut(tag, ":"); ok && name != "" {
			statsTags[name] = struct{}{}
		}
	}

	// Verify all expected tags are present
	for tag := range expectedTags {
		require.Contains(t, statsTags, tag, "Expected IIS tag %s to be present", tag)
	}
	t.Logf("Successfully captured IIS HTTP traffic: %d requests with IIS tags verified", stat.Count)
	return true
}

// TestHTTPStats tests basic HTTP monitoring functionality on Windows.
func TestHTTPStats(t *testing.T) {
	const (
		expectedStatus = 204
		testPath       = "/test"
	)

	serverPort := tracetestutil.FreeTCPPort(t)
	serverAddr := fmt.Sprintf("127.0.0.1:%d", serverPort)
	t.Logf("Using server address: %s (port: %d)", serverAddr, serverPort)

	srvDoneFn := testutil.HTTPServer(t, serverAddr, testutil.Options{
		EnableKeepAlive: true,
	})
	t.Cleanup(srvDoneFn)

	monitor := setupWindowsMonitor(t, getHTTPCfg())

	resp, err := nethttp.Get("http://" + serverAddr + "/" + strconv.Itoa(nethttp.StatusNoContent) + testPath)
	require.NoError(t, err)
	defer resp.Body.Close()

	srvDoneFn()

	require.Eventuallyf(t, func() bool {
		return verifyHTTPStats(t, monitor, serverPort, testPath, expectedStatus, nil)
	}, 3*time.Second, 100*time.Millisecond, "http connection not found for %s", serverAddr)
}

// TestHTTPStatsWithIIS tests HTTP monitoring with a real IIS server.
// This test requires administrator privileges.
// If IIS is not installed, it will be installed automatically.
func TestHTTPStatsWithIIS(t *testing.T) {
	// Create IIS manager
	iisManager := iistestutil.NewIISManager(t)

	// Ensure IIS is installed
	iisManager.EnsureIISInstalled()

	expectedTags := map[string]struct{}{
		"http.iis.site":     {},
		"http.iis.sitename": {},
		"http.iis.app_pool": {},
	}

	const (
		siteName       = "DatadogTestSite"
		expectedStatus = 200
		testPath       = "/index.html"
		indexContent   = "Hello from IIS test"
	)

	// Get a free port for the IIS site
	serverPort := tracetestutil.FreeTCPPort(t)
	serverAddr := fmt.Sprintf("http://localhost:%d", serverPort)
	t.Logf("Setting up IIS site at: %s (port: %d)", serverAddr, serverPort)

	// Setup IIS site using the manager
	err := iisManager.SetupIISSite(siteName, serverPort, indexContent)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = iisManager.CleanupIISSite(siteName)
	})

	// Setup monitor
	monitor := setupWindowsMonitor(t, getHTTPCfg())

	// Make HTTP GET request to IIS
	t.Logf("Making HTTP GET request to: %s%s", serverAddr, testPath)
	resp, err := nethttp.Get(serverAddr + testPath)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Verify we got a successful response
	require.Equal(t, expectedStatus, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	t.Logf("Response body length: %d bytes", len(body))

	// Verify the monitor captured the HTTP traffic
	require.Eventuallyf(t, func() bool {
		return verifyHTTPStats(t, monitor, serverPort, testPath, expectedStatus, func(t *testing.T, stat *http.RequestStat) bool {
			return verifyIISDynamicTags(t, stat, expectedTags)
		})
	}, 5*time.Second, 100*time.Millisecond, "HTTP connection to IIS not found for %s", serverAddr)
}
