// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows && npm

package usm

import (
	nethttp "net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	tracetestutil "github.com/DataDog/datadog-agent/pkg/trace/testutil"
)

// Windows ETW maps TRACE, CONNECT, PATCH and several other methods to MethodUnknown,
// which causes them to be silently dropped by the statkeeper (see etw_http_service_defs.go).
// Only include methods that ETW actually supports.
var (
	httpMethods         = []string{nethttp.MethodGet, nethttp.MethodHead, nethttp.MethodPost, nethttp.MethodPut, nethttp.MethodDelete, nethttp.MethodOptions}
	httpMethodsWithBody = []string{nethttp.MethodPost, nethttp.MethodPut, nethttp.MethodDelete}
)

// windowsMonitorAdapter wraps the Windows Monitor to implement TestMonitor interface.
type windowsMonitorAdapter struct {
	monitor Monitor
}

// GetHTTPStats implements TestMonitor interface for Windows.
func (a *windowsMonitorAdapter) GetHTTPStats() map[http.Key]*http.RequestStats {
	allStats := a.monitor.GetHTTPStats()
	if allStats == nil {
		return nil
	}
	stats, ok := allStats[protocols.HTTP].(map[http.Key]*http.RequestStats)
	if !ok {
		return nil
	}
	return stats
}

// setupWindowsTestMonitor creates a Windows monitor wrapped as TestMonitor.
func setupWindowsTestMonitor(t *testing.T, cfg *config.Config) TestMonitor {
	monitor := setupWindowsMonitor(t, cfg)
	return &windowsMonitorAdapter{
		monitor: monitor,
	}
}

// newWindowsCommonTestParams creates commonTestParams for Windows with a free port.
func newWindowsCommonTestParams(t *testing.T) commonTestParams {
	return commonTestParams{
		serverPort: tracetestutil.FreeTCPPort(t),
		setupMonitor: func(t *testing.T) TestMonitor {
			return setupWindowsTestMonitor(t, getHTTPCfg())
		},
		expectedOccurrences: 2,
	}
}

func TestHTTPStatsCommon(t *testing.T) {
	runHTTPStatsTest(t, newWindowsCommonTestParams(t))
}

func TestHTTPMonitorIntegrationWithResponseBodyCommon(t *testing.T) {
	runHTTPMonitorIntegrationWithResponseBodyTest(t, newWindowsCommonTestParams(t))
}

func TestHTTPMonitorLoadWithIncompleteBuffersCommon(t *testing.T) {
	runHTTPMonitorLoadWithIncompleteBuffersTest(t, httpLoadTestParams{
		slowServerPort: tracetestutil.FreeTCPPort(t),
		fastServerPort: tracetestutil.FreeTCPPort(t),
		setupMonitor: func(t *testing.T) TestMonitor {
			return setupWindowsTestMonitor(t, getHTTPCfg())
		},
		expectedOccurrences: 2,
	})
}

func TestRSTPacketRegressionCommon(t *testing.T) {
	runRSTPacketRegressionTest(t, newWindowsCommonTestParams(t))
}

func TestKeepAliveWithIncompleteResponseRegressionCommon(t *testing.T) {
	runKeepAliveWithIncompleteResponseRegressionTest(t, newWindowsCommonTestParams(t))
}

func TestEmptyConfigCommon(t *testing.T) {
	runEmptyConfigTest(t, emptyConfigTestParams{
		validateMonitorCreation: func(t *testing.T) {
			cfg := NewUSMEmptyConfig()
			monitor := setupWindowsMonitor(t, cfg)
			require.NotNil(t, monitor, "Windows monitor should be created even with empty config")
		},
	})
}
