// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"testing"

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

	serverPort := tracetestutil.FreeTCPPort(t)

	runHTTPMonitorIntegrationWithResponseBodyTest(t, httpBodySizeTestParams{
		serverPort: serverPort,
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
