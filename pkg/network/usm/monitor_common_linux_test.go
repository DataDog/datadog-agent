// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	tracetestutil "github.com/DataDog/datadog-agent/pkg/trace/testutil"
)

// linuxMonitorAdapter wraps the Linux Monitor to implement TestMonitor interface.
type linuxMonitorAdapter struct {
	monitor *Monitor
	t       *testing.T
}

// GetHTTPStats implements TestMonitor interface for Linux.
func (a *linuxMonitorAdapter) GetHTTPStats() map[protocols.ProtocolType]interface{} {
	statsObj, cleaners := a.monitor.GetProtocolStats()
	a.t.Cleanup(cleaners)
	return statsObj
}

// setupLinuxTestMonitor creates a Linux monitor wrapped as TestMonitor.
func setupLinuxTestMonitor(t *testing.T, cfg *config.Config) TestMonitor {
	monitor := setupUSMTLSMonitor(t, cfg, useExistingConsumer)
	return &linuxMonitorAdapter{
		monitor: monitor,
		t:       t,
	}
}

// TestHTTPStatsCommon runs the common HTTP stats test on Linux.
func TestHTTPStatsCommon(t *testing.T) {
	skipTestIfKernelNotSupported(t)

	serverPort := tracetestutil.FreeTCPPort(t)

	runHTTPStatsTest(t, httpStatsTestParams{
		serverPort: serverPort,
		setupMonitor: func(t *testing.T) TestMonitor {
			return setupLinuxTestMonitor(t, getHTTPCfg())
		},
	})
}

// TestHTTPMonitorIntegrationWithResponseBodyCommon runs the HTTP body size test on Linux.
func TestHTTPMonitorIntegrationWithResponseBodyCommon(t *testing.T) {
	skipTestIfKernelNotSupported(t)

	runHTTPMonitorIntegrationWithResponseBodyTest(t, httpBodySizeTestParams{
		setupMonitor: func(t *testing.T) TestMonitor {
			return setupLinuxTestMonitor(t, getHTTPCfg())
		},
	})
}

// TestHTTPMonitorLoadWithIncompleteBuffersCommon runs the incomplete buffers test on Linux.
func TestHTTPMonitorLoadWithIncompleteBuffersCommon(t *testing.T) {
	skipTestIfKernelNotSupported(t)

	slowServerPort := tracetestutil.FreeTCPPort(t)
	fastServerPort := tracetestutil.FreeTCPPort(t)

	runHTTPMonitorLoadWithIncompleteBuffersTest(t, httpLoadTestParams{
		slowServerPort: slowServerPort,
		fastServerPort: fastServerPort,
		setupMonitor: func(t *testing.T) TestMonitor {
			return setupLinuxTestMonitor(t, getHTTPCfg())
		},
	})
}

// TestRSTPacketRegressionCommon runs the RST packet regression test on Linux.
func TestRSTPacketRegressionCommon(t *testing.T) {
	skipTestIfKernelNotSupported(t)

	serverPort := tracetestutil.FreeTCPPort(t)

	runRSTPacketRegressionTest(t, rstPacketTestParams{
		serverPort: serverPort,
		setupMonitor: func(t *testing.T) TestMonitor {
			return setupLinuxTestMonitor(t, getHTTPCfg())
		},
	})
}

// TestKeepAliveWithIncompleteResponseRegressionCommon runs the keep-alive with incomplete response test on Linux.
func TestKeepAliveWithIncompleteResponseRegressionCommon(t *testing.T) {
	skipTestIfKernelNotSupported(t)

	serverPort := tracetestutil.FreeTCPPort(t)

	runKeepAliveWithIncompleteResponseRegressionTest(t, keepAliveWithIncompleteResponseTestParams{
		serverPort: serverPort,
		setupMonitor: func(t *testing.T) TestMonitor {
			return setupLinuxTestMonitor(t, getHTTPCfg())
		},
	})
}

// TestEmptyConfigCommon runs the empty config test on Linux.
func TestEmptyConfigCommon(t *testing.T) {
	skipTestIfKernelNotSupported(t)

	runEmptyConfigTest(t, emptyConfigTestParams{
		validateMonitorCreation: func(t *testing.T) {
			cfg := NewUSMEmptyConfig()
			// On Linux, the monitor should not start and not return an error
			// when no protocols are enabled.
			monitor, err := NewMonitor(cfg, nil, nil)
			require.Nil(t, monitor, "monitor should be nil when no protocols are enabled")
			require.NoError(t, err, "should not return error when no protocols are enabled")
		},
	})
}
