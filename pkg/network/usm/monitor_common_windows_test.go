// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package usm

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	tracetestutil "github.com/DataDog/datadog-agent/pkg/trace/testutil"
)

// windowsMonitorAdapter wraps the Windows Monitor to implement TestMonitor interface.
// Note: Windows Monitor interface already has GetHTTPStats(), so this is a simple wrapper.
type windowsMonitorAdapter struct {
	monitor Monitor
}

// GetHTTPStats implements TestMonitor interface for Windows.
func (a *windowsMonitorAdapter) GetHTTPStats() map[protocols.ProtocolType]interface{} {
	return a.monitor.GetHTTPStats()
}

// setupWindowsTestMonitor creates a Windows monitor wrapped as TestMonitor.
func setupWindowsTestMonitor(t *testing.T, cfg *config.Config) TestMonitor {
	monitor := setupWindowsMonitor(t, cfg)
	return &windowsMonitorAdapter{
		monitor: monitor,
	}
}

// TestHTTPStatsCommon runs the common HTTP stats test on Windows.
func TestHTTPStatsCommon(t *testing.T) {
	serverPort := tracetestutil.FreeTCPPort(t)

	runHTTPStatsTest(t, httpStatsTestParams{
		serverPort: serverPort,
		setupMonitor: func(t *testing.T) TestMonitor {
			return setupWindowsTestMonitor(t, getHTTPCfg())
		},
	})
}

// TestHTTPMonitorIntegrationWithResponseBodyCommon runs the HTTP body size test on Windows.
func TestHTTPMonitorIntegrationWithResponseBodyCommon(t *testing.T) {
	runHTTPMonitorIntegrationWithResponseBodyTest(t, httpBodySizeTestParams{
		setupMonitor: func(t *testing.T) TestMonitor {
			return setupWindowsTestMonitor(t, getHTTPCfg())
		},
	})
}

// TestHTTPMonitorLoadWithIncompleteBuffersCommon runs the incomplete buffers test on Windows.
func TestHTTPMonitorLoadWithIncompleteBuffersCommon(t *testing.T) {
	slowServerPort := tracetestutil.FreeTCPPort(t)
	fastServerPort := tracetestutil.FreeTCPPort(t)

	runHTTPMonitorLoadWithIncompleteBuffersTest(t, httpLoadTestParams{
		slowServerPort: slowServerPort,
		fastServerPort: fastServerPort,
		setupMonitor: func(t *testing.T) TestMonitor {
			return setupWindowsTestMonitor(t, getHTTPCfg())
		},
	})
}

// TestRSTPacketRegressionCommon runs the RST packet regression test on Windows.
func TestRSTPacketRegressionCommon(t *testing.T) {
	serverPort := tracetestutil.FreeTCPPort(t)

	runRSTPacketRegressionTest(t, rstPacketTestParams{
		serverPort: serverPort,
		setupMonitor: func(t *testing.T) TestMonitor {
			return setupWindowsTestMonitor(t, getHTTPCfg())
		},
	})
}

// TestKeepAliveWithIncompleteResponseRegressionCommon runs the keep-alive with incomplete response test on Windows.
func TestKeepAliveWithIncompleteResponseRegressionCommon(t *testing.T) {
	serverPort := tracetestutil.FreeTCPPort(t)

	runKeepAliveWithIncompleteResponseRegressionTest(t, keepAliveWithIncompleteResponseTestParams{
		serverPort: serverPort,
		setupMonitor: func(t *testing.T) TestMonitor {
			return setupWindowsTestMonitor(t, getHTTPCfg())
		},
	})
}

// TestEmptyConfigCommon runs the empty config test on Windows.
func TestEmptyConfigCommon(t *testing.T) {
	runEmptyConfigTest(t, emptyConfigTestParams{
		validateMonitorCreation: func(t *testing.T) {
			// On Windows, the monitor is always created even with empty config.
			// It just configures which protocols to capture via SetCapturedProtocols.
			// We verify this by creating a monitor with empty config and checking it's not nil.
			cfg := NewUSMEmptyConfig()
			monitor := setupWindowsMonitor(t, cfg)
			require.NotNil(t, monitor, "Windows monitor should be created even with empty config")
		},
	})
}
