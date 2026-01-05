// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows && npm && test

package usm

import (
	"fmt"
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

			if stat := reqStats.Data[expectedStatus]; stat != nil && stat.Count == 1 {
				return true
			}
		}

		return false
	}, 3*time.Second, 100*time.Millisecond, "http connection not found for %s", serverAddr)
}
