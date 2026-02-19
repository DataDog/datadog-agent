// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	nethttp "net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	usmconfig "github.com/DataDog/datadog-agent/pkg/network/usm/config"
	usmtestutil "github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
	tracetestutil "github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
)

// Linux eBPF supports all standard HTTP methods.
var (
	httpMethods         = []string{nethttp.MethodGet, nethttp.MethodHead, nethttp.MethodPost, nethttp.MethodPut, nethttp.MethodPatch, nethttp.MethodDelete, nethttp.MethodOptions, nethttp.MethodTrace}
	httpMethodsWithBody = []string{nethttp.MethodPost, nethttp.MethodPut, nethttp.MethodPatch, nethttp.MethodDelete}
)

// linuxMonitorAdapter wraps the Linux Monitor to implement TestMonitor interface.
type linuxMonitorAdapter struct {
	monitor *Monitor
	t       *testing.T
}

// GetHTTPStats implements TestMonitor interface for Linux.
func (a *linuxMonitorAdapter) GetHTTPStats() map[http.Key]*http.RequestStats {
	allStats, cleaners := a.monitor.GetProtocolStats()
	a.t.Cleanup(cleaners)
	if allStats == nil {
		return nil
	}
	stats, ok := allStats[protocols.HTTP].(map[http.Key]*http.RequestStats)
	if !ok {
		return nil
	}
	return stats
}

// setupLinuxTestMonitor creates a Linux monitor wrapped as TestMonitor.
func setupLinuxTestMonitor(t *testing.T, cfg *config.Config) TestMonitor {
	monitor := setupUSMTLSMonitor(t, cfg, useExistingConsumer)
	return &linuxMonitorAdapter{
		monitor: monitor,
		t:       t,
	}
}

// newLinuxCommonTestParams creates commonTestParams for Linux with a free port.
func newLinuxCommonTestParams(t *testing.T) commonTestParams {
	return commonTestParams{
		serverPort: tracetestutil.FreeTCPPort(t),
		setupMonitor: func(t *testing.T) TestMonitor {
			return setupLinuxTestMonitor(t, getHTTPCfg())
		},
		expectedOccurrences: 1,
	}
}

// HTTPCommonTestSuite runs the common HTTP tests under multiple eBPF build modes.
type HTTPCommonTestSuite struct {
	suite.Suite
}

func TestHTTPCommon(t *testing.T) {
	if kernel.MustHostVersion() < usmconfig.MinimumKernelVersion {
		t.Skipf("USM is not supported on %v", kernel.MustHostVersion())
	}
	ebpftest.TestBuildModes(t, usmtestutil.SupportedBuildModes(), "", func(t *testing.T) {
		suite.Run(t, new(HTTPCommonTestSuite))
	})
}

func (s *HTTPCommonTestSuite) TestHTTPStats() {
	runHTTPStatsTest(s.T(), newLinuxCommonTestParams(s.T()))
}

func (s *HTTPCommonTestSuite) TestHTTPMonitorIntegrationWithResponseBody() {
	flake.MarkOnJobName(s.T(), "ubuntu_25.10")
	runHTTPMonitorIntegrationWithResponseBodyTest(s.T(), newLinuxCommonTestParams(s.T()))
}

func (s *HTTPCommonTestSuite) TestHTTPMonitorLoadWithIncompleteBuffers() {
	t := s.T()
	runHTTPMonitorLoadWithIncompleteBuffersTest(t, httpLoadTestParams{
		slowServerPort: tracetestutil.FreeTCPPort(t),
		fastServerPort: tracetestutil.FreeTCPPort(t),
		setupMonitor: func(t *testing.T) TestMonitor {
			return setupLinuxTestMonitor(t, getHTTPCfg())
		},
		expectedOccurrences: 1,
	})
}

func (s *HTTPCommonTestSuite) TestRSTPacketRegression() {
	runRSTPacketRegressionTest(s.T(), newLinuxCommonTestParams(s.T()))
}

func (s *HTTPCommonTestSuite) TestKeepAliveWithIncompleteResponseRegression() {
	runKeepAliveWithIncompleteResponseRegressionTest(s.T(), newLinuxCommonTestParams(s.T()))
}

// TestEmptyConfigCommon is a standalone test (not part of the suite) matching the original.
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
