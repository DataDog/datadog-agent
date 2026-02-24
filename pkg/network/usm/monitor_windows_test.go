// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows && npm

package usm

import (
	"fmt"
	"io"
	nethttp "net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	iistestutil "github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
	tracetestutil "github.com/DataDog/datadog-agent/pkg/trace/testutil"
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

// statusCodeCount holds the expected status code and count for validation.
type statusCodeCount struct {
	statusCode uint16
	count      int
}

// getHTTPLikeProtocolStats extracts HTTP protocol stats from the monitor.
func getHTTPLikeProtocolStats(t *testing.T, monitor Monitor, protocolType protocols.ProtocolType) map[http.Key]*http.RequestStats {
	t.Helper()

	allStats := monitor.GetHTTPStats()
	if allStats == nil {
		return nil
	}

	statsObj, ok := allStats[protocolType]
	if !ok {
		return nil
	}

	stats, ok := statsObj.(map[http.Key]*http.RequestStats)
	if !ok {
		return nil
	}

	return stats
}

// verifyHTTPStats validates that the expected HTTP endpoints are present in the stats.
// expectedEndpoints maps http.Key (without connection details) to expected status code and count.
// serverPort is used to filter stats to only those matching the server port.
// additionalValidator is optional - if provided, performs custom validation on each RequestStat.
// Returns true if all expected endpoints are found with matching status codes and counts.
func verifyHTTPStats(t *testing.T, monitor Monitor, expectedEndpoints map[http.Key]statusCodeCount, serverPort int, additionalValidator func(*testing.T, *http.RequestStat) bool) bool {
	t.Helper()

	stats := getHTTPLikeProtocolStats(t, monitor, protocols.HTTP)
	if len(stats) == 0 {
		return false
	}

	// Build result map from actual stats
	result := make(map[http.Key]statusCodeCount)

	for key, reqStats := range stats {
		// Only check stats matching the server port
		if key.SrcPort != uint16(serverPort) && key.DstPort != uint16(serverPort) {
			continue
		}

		// Iterate through all status codes in the stats
		for statusCode, stat := range reqStats.Data {
			if stat == nil || stat.Count == 0 {
				continue
			}

			// Run additional validation if provided
			if additionalValidator != nil && !additionalValidator(t, stat) {
				continue
			}

			// Create a simplified key for comparison (normalize path and method only)
			simpleKey := http.Key{
				Method: key.Method,
				Path: http.Path{
					Content: key.Path.Content,
				},
			}

			// Store in result map
			result[simpleKey] = statusCodeCount{
				statusCode: statusCode,
				count:      stat.Count,
			}
		}
	}

	// Compare result with expected endpoints
	if len(result) != len(expectedEndpoints) {
		return false
	}

	for key, expected := range expectedEndpoints {
		actual, ok := result[key]
		if !ok {
			return false
		}
		if actual.statusCode != expected.statusCode || actual.count != expected.count {
			return false
		}
	}

	return true
}

// makeIISTagValidator creates a validator function that checks for expected IIS dynamic tags.
func makeIISTagValidator(expectedTags map[string]struct{}) func(*testing.T, *http.RequestStat) bool {
	return func(t *testing.T, stat *http.RequestStat) bool {
		if stat.DynamicTags == nil {
			return false
		}

		statsTags := make(map[string]struct{})
		for _, tag := range stat.DynamicTags.GetAll() {
			if name, _, ok := strings.Cut(tag, ":"); ok && name != "" {
				statsTags[name] = struct{}{}
			}
		}

		// Check if all expected tags are present
		for expectedTag := range expectedTags {
			if _, exists := statsTags[expectedTag]; !exists {
				return false
			}
		}

		t.Logf("Successfully captured IIS HTTP traffic: %d requests with IIS tags verified", stat.Count)
		return true
	}
}

// TestHTTPStats tests basic HTTP monitoring functionality on Windows.
func TestHTTPStats(t *testing.T) {
	serverPort := tracetestutil.FreeTCPPort(t)
	serverAddr := fmt.Sprintf("127.0.0.1:%d", serverPort)
	t.Logf("Using server address: %s (port: %d)", serverAddr, serverPort)

	srvDoneFn := testutil.HTTPServer(t, serverAddr, testutil.Options{
		EnableKeepAlive: true,
	})
	t.Cleanup(srvDoneFn)

	monitor := setupWindowsMonitor(t, getHTTPCfg())

	// Define test endpoints with status codes in path
	testEndpointPath := "/" + strconv.Itoa(nethttp.StatusNoContent) + "/test"
	healthEndpointPath := "/" + strconv.Itoa(nethttp.StatusOK) + "/api/health"

	// Make first request: GET /204/test with 204 status
	resp1, err := nethttp.Get("http://" + serverAddr + testEndpointPath)
	require.NoError(t, err)
	defer resp1.Body.Close()

	// Make second request: GET /200/api/health with 200 status
	resp2, err := nethttp.Get("http://" + serverAddr + healthEndpointPath)
	require.NoError(t, err)
	defer resp2.Body.Close()

	srvDoneFn()

	// Define expected endpoints
	expectedEndpoints := map[http.Key]statusCodeCount{
		{
			Path:   http.Path{Content: http.Interner.GetString(testEndpointPath)},
			Method: http.MethodGet,
		}: {
			statusCode: 204,
			count:      1,
		},
		{
			Path:   http.Path{Content: http.Interner.GetString(healthEndpointPath)},
			Method: http.MethodGet,
		}: {
			statusCode: 200,
			count:      1,
		},
	}

	// Verify both endpoints were captured by the monitor
	require.Eventuallyf(t, func() bool {
		return verifyHTTPStats(t, monitor, expectedEndpoints, serverPort, nil)
	}, 3*time.Second, 100*time.Millisecond, "HTTP connections not found for %s", serverAddr)
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
		"http.iis.subsite":  {},
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

	// Define expected endpoints
	expectedEndpoints := map[http.Key]statusCodeCount{
		{
			Path:   http.Path{Content: http.Interner.GetString(testPath)},
			Method: http.MethodGet,
		}: {
			statusCode: uint16(expectedStatus),
			count:      1,
		},
	}

	// Verify the monitor captured the HTTP traffic with IIS tags
	require.Eventuallyf(t, func() bool {
		return verifyHTTPStats(t, monitor, expectedEndpoints, serverPort, makeIISTagValidator(expectedTags))
	}, 5*time.Second, 100*time.Millisecond, "HTTP connection to IIS not found for %s", serverAddr)
}
