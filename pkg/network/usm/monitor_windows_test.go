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
	"slices"
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

// HTTPEndpointValidator defines a function that checks if an HTTP endpoint is relevant and valid.
// It receives the request key and stats, and should return true if this endpoint matches the expected criteria.
type HTTPEndpointValidator func(t *testing.T, key http.Key, reqStats *http.RequestStats) bool

// makeHTTPEndpointValidator creates a validator function for a specific HTTP endpoint.
// It checks for GET requests matching the path suffix, port, status code, and expected count.
// Optional additionalChecks can be provided for extra validations (e.g., IIS tags).
func makeHTTPEndpointValidator(pathSuffix string, expectedStatus uint16, serverPort int, expectedCount int, additionalChecks func(*testing.T, *http.RequestStat) bool) HTTPEndpointValidator {
	return func(t *testing.T, key http.Key, reqStats *http.RequestStats) bool {
		// Check method
		if key.Method != http.MethodGet {
			return false
		}
		// Check path
		if !strings.HasSuffix(key.Path.Content.Get(), pathSuffix) {
			return false
		}
		// Check port (either source or destination should match server port)
		if key.SrcPort != uint16(serverPort) && key.DstPort != uint16(serverPort) {
			return false
		}

		// Check status code and count
		stat := reqStats.Data[expectedStatus]
		if stat == nil || stat.Count != expectedCount {
			return false
		}

		// Run any additional checks
		if additionalChecks != nil {
			return additionalChecks(t, stat)
		}
		return true
	}
}

// verifyHTTPStats iterates through HTTP stats and checks which of the expected endpoints are found.
// Each validator in expectedEndpoints determines if an endpoint is "relevant" (matches the criteria) and passes validation.
// endpointsFound is a slice where each index corresponds to an endpoint, and the value indicates if found.
// This function is called repeatedly in require.Eventually to handle in-flight kernel requests.
func verifyHTTPStats(t *testing.T, monitor Monitor, endpointsFound []bool, expectedEndpoints ...HTTPEndpointValidator) {
	t.Helper()

	// Get fresh stats from monitor
	allStats := monitor.GetHTTPStats()
	if allStats == nil {
		return
	}

	statsObj, ok := allStats[protocols.HTTP]
	if !ok {
		return
	}

	stats, ok := statsObj.(map[http.Key]*http.RequestStats)
	if !ok {
		t.Logf("unexpected type for HTTP stats: %T", statsObj)
		return
	}

	// Iterate through all HTTP stats and check validators
	for key, reqStats := range stats {
		// Check against each expected endpoint validator
		for i, validator := range expectedEndpoints {
			// Only check validators that haven't been satisfied yet
			if !endpointsFound[i] && validator(t, key, reqStats) {
				t.Logf("Found expected endpoint %d: %v %s [%d:%d]", i, key.Method, key.Path.Content.Get(), key.SrcPort, key.DstPort)
				endpointsFound[i] = true
			}
		}
	}
}

// allEndpointsFound is a helper that checks if all expected endpoints were found.
func allEndpointsFound(found []bool) bool {
	return !slices.Contains(found, false)
}

// verifyIISDynamicTags validates that IIS-specific dynamic tags are present in the request stats.
func verifyIISDynamicTags(t *testing.T, stat *http.RequestStat, expectedTags map[string]struct{}) {
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
	require.Subset(t, expectedTags, statsTags, "Expected IIS tags to be present in stats")
	t.Logf("Successfully captured IIS HTTP traffic: %d requests with IIS tags verified", stat.Count)
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

	// Make first request: GET /test with 204 status
	resp1, err := nethttp.Get("http://" + serverAddr + "/" + strconv.Itoa(nethttp.StatusNoContent) + "/test")
	require.NoError(t, err)
	defer resp1.Body.Close()

	// Make second request: GET /api/health with 200 status
	resp2, err := nethttp.Get("http://" + serverAddr + "/" + strconv.Itoa(nethttp.StatusOK) + "/api/health")
	require.NoError(t, err)
	defer resp2.Body.Close()

	srvDoneFn()

	// Verify both endpoints were captured by the monitor
	// Track which endpoints have been found across iterations to handle in-flight kernel requests
	endpointsFound := make([]bool, 2)
	require.Eventuallyf(t, func() bool {
		verifyHTTPStats(t, monitor, endpointsFound,
			makeHTTPEndpointValidator("/test", 204, serverPort, 1, nil),
			makeHTTPEndpointValidator("/api/health", 200, serverPort, 1, nil),
		)
		return allEndpointsFound(endpointsFound)
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
	// Track which endpoints have been found across iterations to handle in-flight kernel requests
	endpointsFound := make([]bool, 1)
	require.Eventuallyf(t, func() bool {
		verifyHTTPStats(t, monitor, endpointsFound,
			makeHTTPEndpointValidator(testPath, uint16(expectedStatus), serverPort, 1, func(t *testing.T, stat *http.RequestStat) bool {
				// Verify IIS-specific dynamic tags
				verifyIISDynamicTags(t, stat, expectedTags)
				return true
			}),
		)
		return allEndpointsFound(endpointsFound)
	}, 5*time.Second, 100*time.Millisecond, "HTTP connection to IIS not found for %s", serverAddr)
}
