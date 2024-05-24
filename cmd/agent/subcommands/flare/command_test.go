// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const sysprobeSockPath = "/tmp/sysprobe.sock"

func getPprofTestServer(t *testing.T) (tcpServer *httptest.Server, unixServer *httptest.Server) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/debug/pprof/heap":
			w.Write([]byte("heap_profile"))
		case "/debug/pprof/profile":
			time := r.URL.Query()["seconds"][0]
			w.Write([]byte(time + "_sec_cpu_pprof"))
		case "/debug/pprof/mutex":
			w.Write([]byte("mutex"))
		case "/debug/pprof/block":
			w.Write([]byte("block"))
		case "/debug/stats": // only for system-probe
			w.WriteHeader(200)
		case "/debug/pprof/trace":
			w.Write([]byte("trace"))
		default:
			w.WriteHeader(500)
		}
	})

	tcpServer = httptest.NewServer(handler)
	if runtime.GOOS == "linux" {
		unixServer = httptest.NewUnstartedServer(handler)
		var err error
		unixServer.Listener, err = net.Listen("unix", sysprobeSockPath)
		require.NoError(t, err, "could not create listener for unix socket /tmp/sysprobe.sock")
		unixServer.Start()

		return tcpServer, unixServer
	}

	return tcpServer, tcpServer
}

func TestReadProfileData(t *testing.T) {
	ts, uts := getPprofTestServer(t)
	t.Cleanup(func() {
		ts.Close()
		uts.Close()
	})

	u, err := url.Parse(ts.URL)
	require.NoError(t, err)
	port := u.Port()

	mockConfig := config.Mock(t)
	mockConfig.SetWithoutSource("expvar_port", port)
	mockConfig.SetWithoutSource("apm_config.enabled", true)
	mockConfig.SetWithoutSource("apm_config.debug.port", port)
	mockConfig.SetWithoutSource("apm_config.receiver_timeout", "10")
	mockConfig.SetWithoutSource("process_config.expvar_port", port)
	mockConfig.SetWithoutSource("security_agent.expvar_port", port)

	mockSysProbeConfig := config.MockSystemProbe(t)
	mockSysProbeConfig.SetWithoutSource("system_probe_config.enabled", true)
	if runtime.GOOS == "windows" {
		mockSysProbeConfig.SetWithoutSource("system_probe_config.sysprobe_socket", u.Host)
	} else {
		mockSysProbeConfig.SetWithoutSource("system_probe_config.sysprobe_socket", sysprobeSockPath)
	}

	data, err := readProfileData(10)
	require.NoError(t, err)

	expected := flare.ProfileData{
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
		maps.Copy(expected, flare.ProfileData{
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

func TestReadProfileDataNoTraceAgent(t *testing.T) {
	ts, uts := getPprofTestServer(t)
	t.Cleanup(func() {
		ts.Close()
		uts.Close()
	})

	u, err := url.Parse(ts.URL)
	require.NoError(t, err)
	port := u.Port()

	mockConfig := config.Mock(t)
	mockConfig.SetWithoutSource("expvar_port", port)
	mockConfig.SetWithoutSource("apm_config.enabled", true)
	mockConfig.SetWithoutSource("apm_config.debug.port", 0)
	mockConfig.SetWithoutSource("apm_config.receiver_timeout", "10")
	mockConfig.SetWithoutSource("process_config.expvar_port", port)
	mockConfig.SetWithoutSource("security_agent.expvar_port", port)

	mockSysProbeConfig := config.MockSystemProbe(t)
	mockSysProbeConfig.SetWithoutSource("system_probe_config.enabled", true)
	if runtime.GOOS == "windows" {
		mockSysProbeConfig.SetWithoutSource("system_probe_config.sysprobe_socket", u.Host)
	} else {
		mockSysProbeConfig.SetWithoutSource("system_probe_config.sysprobe_socket", sysprobeSockPath)
	}

	data, err := readProfileData(10)
	require.Error(t, err)
	require.Regexp(t, "^* error collecting trace agent profile: ", err.Error())

	expected := flare.ProfileData{
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
		maps.Copy(expected, flare.ProfileData{
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

func TestReadProfileDataErrors(t *testing.T) {
	mockConfig := config.Mock(t)
	mockConfig.SetWithoutSource("apm_config.enabled", true)
	mockConfig.SetWithoutSource("apm_config.debug.port", 0)

	data, err := readProfileData(10)
	require.Error(t, err)
	require.Regexp(t, "^4 errors occurred:\n", err.Error())
	require.Len(t, data, 0)
}

func TestCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"flare", "1234"},
		makeFlare,
		func(cliParams *cliParams, coreParams core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, []string{"1234"}, cliParams.args)
			require.Equal(t, true, secretParams.Enabled)
		})
}
