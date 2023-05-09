// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	assert "github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func getPprofTestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		default:
			w.WriteHeader(500)
		}
	}))
}

func TestReadProfileData(t *testing.T) {
	ts := getPprofTestServer()
	defer ts.Close()

	u, err := url.Parse(ts.URL)
	require.NoError(t, err)
	port := u.Port()

	mockConfig := config.Mock(t)
	mockConfig.Set("expvar_port", port)
	mockConfig.Set("apm_config.enabled", true)
	mockConfig.Set("apm_config.debug.port", port)
	mockConfig.Set("apm_config.receiver_timeout", "10")
	mockConfig.Set("process_config.expvar_port", port)
	mockConfig.Set("security_agent.expvar_port", port)
	mockConfig.Set("system_probe_config.debug_port", port)

	data, err := readProfileData(10)
	require.NoError(t, err)

	expected := flare.ProfileData{
		"core-1st-heap.pprof":           []byte("heap_profile"),
		"core-2nd-heap.pprof":           []byte("heap_profile"),
		"core-block.pprof":              []byte("block"),
		"core-cpu.pprof":                []byte("10_sec_cpu_pprof"),
		"core-mutex.pprof":              []byte("mutex"),
		"process-1st-heap.pprof":        []byte("heap_profile"),
		"process-2nd-heap.pprof":        []byte("heap_profile"),
		"process-block.pprof":           []byte("block"),
		"process-cpu.pprof":             []byte("10_sec_cpu_pprof"),
		"process-mutex.pprof":           []byte("mutex"),
		"security-agent-1st-heap.pprof": []byte("heap_profile"),
		"security-agent-2nd-heap.pprof": []byte("heap_profile"),
		"security-agent-block.pprof":    []byte("block"),
		"security-agent-cpu.pprof":      []byte("10_sec_cpu_pprof"),
		"security-agent-mutex.pprof":    []byte("mutex"),
		"trace-1st-heap.pprof":          []byte("heap_profile"),
		"trace-2nd-heap.pprof":          []byte("heap_profile"),
		"trace-block.pprof":             []byte("block"),
		"trace-cpu.pprof":               []byte("10_sec_cpu_pprof"),
		"trace-mutex.pprof":             []byte("mutex"),
		"system-probe-1st-heap.pprof":   []byte("heap_profile"),
		"system-probe-2nd-heap.pprof":   []byte("heap_profile"),
		"system-probe-block.pprof":      []byte("block"),
		"system-probe-cpu.pprof":        []byte("10_sec_cpu_pprof"),
		"system-probe-mutex.pprof":      []byte("mutex"),
	}

	assert.Len(t, data, len(expected), "expected pprof data has more or less profiles than expected")
	for name := range expected {
		assert.Equal(t, expected[name], data[name])
	}
}

func TestReadProfileDataNoTraceAgent(t *testing.T) {
	ts := getPprofTestServer()
	defer ts.Close()

	u, err := url.Parse(ts.URL)
	require.NoError(t, err)
	port := u.Port()

	// We're not setting "apm_config.debug.port" on purpose
	mockConfig := config.Mock(t)
	mockConfig.Set("expvar_port", port)
	mockConfig.Set("apm_config.enabled", true)
	mockConfig.Set("apm_config.receiver_timeout", "10")
	mockConfig.Set("process_config.expvar_port", port)
	mockConfig.Set("security_agent.expvar_port", port)
	mockConfig.Set("system_probe_config.debug_port", port)

	data, err := readProfileData(10)
	require.Error(t, err)
	assert.Regexp(t, "^* error collecting trace agent profile: ", err.Error())

	expected := flare.ProfileData{
		"core-1st-heap.pprof":           []byte("heap_profile"),
		"core-2nd-heap.pprof":           []byte("heap_profile"),
		"core-block.pprof":              []byte("block"),
		"core-cpu.pprof":                []byte("10_sec_cpu_pprof"),
		"core-mutex.pprof":              []byte("mutex"),
		"process-1st-heap.pprof":        []byte("heap_profile"),
		"process-2nd-heap.pprof":        []byte("heap_profile"),
		"process-block.pprof":           []byte("block"),
		"process-cpu.pprof":             []byte("10_sec_cpu_pprof"),
		"process-mutex.pprof":           []byte("mutex"),
		"security-agent-1st-heap.pprof": []byte("heap_profile"),
		"security-agent-2nd-heap.pprof": []byte("heap_profile"),
		"security-agent-block.pprof":    []byte("block"),
		"security-agent-cpu.pprof":      []byte("10_sec_cpu_pprof"),
		"security-agent-mutex.pprof":    []byte("mutex"),
		"system-probe-1st-heap.pprof":   []byte("heap_profile"),
		"system-probe-2nd-heap.pprof":   []byte("heap_profile"),
		"system-probe-block.pprof":      []byte("block"),
		"system-probe-cpu.pprof":        []byte("10_sec_cpu_pprof"),
		"system-probe-mutex.pprof":      []byte("mutex"),
	}

	assert.Len(t, data, len(expected), "expected pprof data has more or less profiles than expected")
	for name := range expected {
		assert.Equal(t, expected[name], data[name])
	}
}

func TestReadProfileDataErrors(t *testing.T) {
	// We're not setting "apm_config.debug.port" on purpose
	mockConfig := config.Mock(t)
	mockConfig.Set("apm_config.enabled", true)

	data, err := readProfileData(10)
	require.Error(t, err)
	assert.Regexp(t, "^4 errors occurred:\n", err.Error())
	assert.Len(t, data, 0)
}

func TestCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"flare", "1234"},
		makeFlare,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, []string{"1234"}, cliParams.args)
			require.Equal(t, true, coreParams.ConfigLoadSecrets())
		})
}
