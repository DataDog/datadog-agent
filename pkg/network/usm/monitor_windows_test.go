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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
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
	monitor := setupWindowsTestMonitor(t, getHTTPCfg())

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
	accumulated := make(map[http.Key]statusCodeCount)
	require.Eventuallyf(t, func() bool {
		return verifyHTTPStats(t, monitor, accumulated, expectedEndpoints, serverPort, 1, makeIISTagValidator(expectedTags))
	}, 5*time.Second, 100*time.Millisecond, "HTTP connection to IIS not found for %s", serverAddr)
}
