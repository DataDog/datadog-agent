// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"maps"
	"net/http/httptest"
	"net/url"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	profiler "github.com/DataDog/datadog-agent/comp/core/profiler/def"
	profilerfx "github.com/DataDog/datadog-agent/comp/core/profiler/fx"
	profilermock "github.com/DataDog/datadog-agent/comp/core/profiler/mock"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/server/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type commandTestSuite struct {
	suite.Suite
	sysprobeSocketPath string
	tcpServer          *httptest.Server
	tcpTLSServer       *httptest.Server
	systemProbeServer  *httptest.Server
}

func (c *commandTestSuite) SetupSuite() {
	t := c.T()
	c.sysprobeSocketPath = testutil.SystemProbeSocketPath(t, "flare")
}

// startTestServers starts test servers from a clean state to ensure no cache responses are used.
// This should be called by each test that requires them.
func (c *commandTestSuite) startTestServers() {
	t := c.T()
	c.tcpServer, c.tcpTLSServer, c.systemProbeServer = c.getPprofTestServer()

	t.Cleanup(func() {
		if c.tcpServer != nil {
			c.tcpServer.Close()
			c.tcpServer = nil
		}
		if c.tcpTLSServer != nil {
			c.tcpTLSServer.Close()
			c.tcpTLSServer = nil
		}
		if c.systemProbeServer != nil {
			c.systemProbeServer.Close()
			c.systemProbeServer = nil
		}
	})
}

func (c *commandTestSuite) getPprofTestServer() (tcpServer *httptest.Server, tcpTLSServer *httptest.Server, sysProbeServer *httptest.Server) {
	var err error

	mockIPC := ipcmock.New(c.T())

	handler := profilermock.NewMockHandler()
	tcpServer = httptest.NewServer(handler)
	tcpTLSServer = mockIPC.NewMockServer(handler)

	sysProbeServer, err = testutil.NewSystemProbeTestServer(handler, c.sysprobeSocketPath)
	require.NoError(c.T(), err, "could not restart system probe server")
	if sysProbeServer != nil {
		sysProbeServer.Start()
	}

	return tcpServer, tcpTLSServer, sysProbeServer
}

type deps struct {
	fx.In

	Profiler profiler.Component
}

func TestCommandTestSuite(t *testing.T) {
	suite.Run(t, &commandTestSuite{})
}

func getProfiler(t testing.TB, mockConfig model.Config, mockSysProbeConfig model.Config) profiler.Component {
	deps := fxutil.Test[deps](
		t,
		core.MockBundle(),
		fx.Replace(configComponent.MockParams{
			Overrides: mockConfig.AllSettings(),
		}),
		fx.Replace(sysprobeconfigimpl.MockParams{
			Overrides: mockSysProbeConfig.AllSettings(),
		}),
		settingsimpl.MockModule(),
		profilerfx.Module(),
		fx.Provide(func() ipc.Component { return ipcmock.New(t) }),
		fx.Provide(func(ipcomp ipc.Component) ipc.HTTPClient { return ipcomp.GetClient() }),
	)

	return deps.Profiler
}

func (c *commandTestSuite) TestReadProfileData() {
	t := c.T()
	c.startTestServers()

	u, err := url.Parse(c.tcpServer.URL)
	require.NoError(t, err)
	port := u.Port()

	u, err = url.Parse(c.tcpTLSServer.URL)
	require.NoError(t, err)
	httpsPort := u.Port()

	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("expvar_port", port)
	mockConfig.SetWithoutSource("apm_config.enabled", true)
	mockConfig.SetWithoutSource("apm_config.debug.port", httpsPort)
	mockConfig.SetWithoutSource("apm_config.receiver_timeout", "10")
	mockConfig.SetWithoutSource("process_config.expvar_port", port)
	mockConfig.SetWithoutSource("security_agent.expvar_port", port)

	mockSysProbeConfig := configmock.NewSystemProbe(t)
	if runtime.GOOS != "darwin" {
		mockSysProbeConfig.SetWithoutSource("system_probe_config.enabled", true)
		mockSysProbeConfig.SetWithoutSource("system_probe_config.sysprobe_socket", c.sysprobeSocketPath)
		mockSysProbeConfig.SetWithoutSource("network_config.enabled", true)
	}

	profiler := getProfiler(t, mockConfig, mockSysProbeConfig)
	data, err := profiler.ReadProfileData(10, func(string, ...interface{}) error { return nil })
	require.NoError(t, err)

	expected := flaretypes.ProfileData{
		"core-1st-heap.pprof":           []byte("heap_profile"),
		"core-2nd-heap.pprof":           []byte("heap_profile"),
		"core-block.pprof":              []byte("block"),
		"core-cpu.pprof":                []byte("10_sec_cpu_pprof"),
		"core-mutex.pprof":              []byte("mutex"),
		"core.trace":                    []byte("trace"),
		"process-1st-heap.pprof":        []byte("heap_profile"),
		"process-2nd-heap.pprof":        []byte("heap_profile"),
		"process-block.pprof":           []byte("block"),
		"process-cpu.pprof":             []byte("10_sec_cpu_pprof"),
		"process-mutex.pprof":           []byte("mutex"),
		"process.trace":                 []byte("trace"),
		"security-agent-1st-heap.pprof": []byte("heap_profile"),
		"security-agent-2nd-heap.pprof": []byte("heap_profile"),
		"security-agent-block.pprof":    []byte("block"),
		"security-agent-cpu.pprof":      []byte("10_sec_cpu_pprof"),
		"security-agent-mutex.pprof":    []byte("mutex"),
		"security-agent.trace":          []byte("trace"),
		"trace-1st-heap.pprof":          []byte("heap_profile"),
		"trace-2nd-heap.pprof":          []byte("heap_profile"),
		"trace-block.pprof":             []byte("block"),
		"trace-cpu.pprof":               []byte("10_sec_cpu_pprof"),
		"trace-mutex.pprof":             []byte("mutex"),
		"trace.trace":                   []byte("trace"),
	}
	if runtime.GOOS != "darwin" {
		maps.Copy(expected, flaretypes.ProfileData{
			"system-probe-1st-heap.pprof": []byte("heap_profile"),
			"system-probe-2nd-heap.pprof": []byte("heap_profile"),
			"system-probe-block.pprof":    []byte("block"),
			"system-probe-cpu.pprof":      []byte("10_sec_cpu_pprof"),
			"system-probe-mutex.pprof":    []byte("mutex"),
			"system-probe.trace":          []byte("trace"),
		})
	}

	require.Len(t, data, len(expected), "expected pprof data has more or less profiles than expected")
	for name := range expected {
		require.Equal(t, expected[name], data[name])
	}
}

func (c *commandTestSuite) TestReadProfileDataNoTraceAgent() {
	t := c.T()
	c.startTestServers()

	u, err := url.Parse(c.tcpServer.URL)
	require.NoError(t, err)
	port := u.Port()

	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("expvar_port", port)
	mockConfig.SetWithoutSource("apm_config.enabled", true)
	mockConfig.SetWithoutSource("apm_config.debug.port", 0)
	mockConfig.SetWithoutSource("apm_config.receiver_timeout", "10")
	mockConfig.SetWithoutSource("process_config.expvar_port", port)
	mockConfig.SetWithoutSource("security_agent.expvar_port", port)

	mockSysProbeConfig := configmock.NewSystemProbe(t)
	if runtime.GOOS != "darwin" {
		mockSysProbeConfig.SetWithoutSource("system_probe_config.enabled", true)
		mockSysProbeConfig.SetWithoutSource("system_probe_config.sysprobe_socket", c.sysprobeSocketPath)
		mockSysProbeConfig.SetWithoutSource("network_config.enabled", true)
	}

	profiler := getProfiler(t, mockConfig, mockSysProbeConfig)
	data, err := profiler.ReadProfileData(10, func(string, ...interface{}) error { return nil })
	require.Error(t, err)
	require.Regexp(t, "^* error collecting trace agent profile: ", err.Error())

	expected := flaretypes.ProfileData{
		"core-1st-heap.pprof":           []byte("heap_profile"),
		"core-2nd-heap.pprof":           []byte("heap_profile"),
		"core-block.pprof":              []byte("block"),
		"core-cpu.pprof":                []byte("10_sec_cpu_pprof"),
		"core-mutex.pprof":              []byte("mutex"),
		"core.trace":                    []byte("trace"),
		"process-1st-heap.pprof":        []byte("heap_profile"),
		"process-2nd-heap.pprof":        []byte("heap_profile"),
		"process-block.pprof":           []byte("block"),
		"process-cpu.pprof":             []byte("10_sec_cpu_pprof"),
		"process-mutex.pprof":           []byte("mutex"),
		"process.trace":                 []byte("trace"),
		"security-agent-1st-heap.pprof": []byte("heap_profile"),
		"security-agent-2nd-heap.pprof": []byte("heap_profile"),
		"security-agent-block.pprof":    []byte("block"),
		"security-agent-cpu.pprof":      []byte("10_sec_cpu_pprof"),
		"security-agent-mutex.pprof":    []byte("mutex"),
		"security-agent.trace":          []byte("trace"),
	}
	if runtime.GOOS != "darwin" {
		maps.Copy(expected, flaretypes.ProfileData{
			"system-probe-1st-heap.pprof": []byte("heap_profile"),
			"system-probe-2nd-heap.pprof": []byte("heap_profile"),
			"system-probe-block.pprof":    []byte("block"),
			"system-probe-cpu.pprof":      []byte("10_sec_cpu_pprof"),
			"system-probe-mutex.pprof":    []byte("mutex"),
			"system-probe.trace":          []byte("trace"),
		})
	}

	require.Len(t, data, len(expected), "expected pprof data has more or less profiles than expected")
	for name := range expected {
		require.Equal(t, expected[name], data[name])
	}
}

func (c *commandTestSuite) TestReadProfileDataErrors() {
	t := c.T()
	c.startTestServers()

	mockConfig := configmock.New(t)
	// setting Core Agent Expvar port to 0 to ensure failing on fetch (using the default value can lead to
	// successful request when running next to an Agent)
	mockConfig.SetWithoutSource("expvar_port", 0)
	mockConfig.SetWithoutSource("security_agent.expvar_port", 0)
	mockConfig.SetWithoutSource("apm_config.enabled", true)
	mockConfig.SetWithoutSource("apm_config.debug.port", 0)
	mockConfig.SetWithoutSource("process_config.enabled", true)
	mockConfig.SetWithoutSource("process_config.expvar_port", 0)

	mockSysProbeConfig := configmock.NewSystemProbe(t)
	InjectConnectionFailures(mockSysProbeConfig, mockConfig)

	profiler := getProfiler(t, mockConfig, mockSysProbeConfig)
	data, err := profiler.ReadProfileData(10, func(string, ...interface{}) error { return nil })

	require.Error(t, err)
	CheckExpectedConnectionFailures(c, err)
	require.Len(t, data, 0)
}

func (c *commandTestSuite) TestCommand() {
	t := c.T()
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"flare", "1234"},
		makeFlare,
		func(cliParams *cliParams, _ core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, []string{"1234"}, cliParams.args)
			require.Equal(t, true, secretParams.Enabled)
		})
}
