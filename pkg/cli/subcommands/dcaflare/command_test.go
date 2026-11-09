// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

package dcaflare

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestResolveDCALogFile(t *testing.T) {
	t.Run("unconfigured log_file falls back to the cluster-agent's default log file", func(t *testing.T) {
		cfg := configmock.New(t)
		require.Equal(t, defaultpaths.GetDefaultDCALogFile(), resolveDCALogFile(cfg))
	})

	t.Run("explicitly configured log_file is preserved", func(t *testing.T) {
		cfg := configmock.New(t)
		cfg.Set("log_file", "/custom/cluster-agent.log", pkgconfigmodel.SourceAgentRuntime)
		assert.Equal(t, "/custom/cluster-agent.log", resolveDCALogFile(cfg))
	})
}

func TestCommand(t *testing.T) {
	commands := []*cobra.Command{
		MakeCommand(func() GlobalParams {
			return GlobalParams{}
		}),
	}

	fxutil.TestOneShotSubcommand(t,
		commands,
		[]string{"flare"},
		run,
		func() {})
}

func serverPort(t *testing.T, serverURL string) int {
	u, err := url.Parse(serverURL)
	require.NoError(t, err)
	_, portStr, err := net.SplitHostPort(u.Host)
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)
	return port
}

func TestRuntimeProfilingSettingsUseConfigEndpoint(t *testing.T) {
	ipc := ipcmock.New(t)

	var mu sync.Mutex
	var paths []string
	server := ipc.NewMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.Path)
		mu.Unlock()

		switch r.URL.Path {
		case "/config/list-runtime":
			w.Write([]byte(`{"runtime_mutex_profile_fraction":{"Description":"","Hidden":false}}`))
		case "/config/runtime_mutex_profile_fraction":
			w.Write([]byte(`{"value":0}`))
		default:
			http.NotFound(w, r)
		}
	}))

	client := newSettingsClient(ipc.GetClient(), serverPort(t, server.URL))
	called := false
	err := settings.ExecWithRuntimeProfilingSettings(func() { called = true }, settings.ProfilingOpts{
		ProfileMutex:         true,
		ProfileMutexFraction: 100,
	}, client)
	require.NoError(t, err)
	assert.True(t, called)

	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, paths, "GET /config/runtime_mutex_profile_fraction")
	assert.Contains(t, paths, "POST /config/runtime_mutex_profile_fraction")
}

func TestReadProfileData(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.RequestURI())
		mu.Unlock()
		w.Write([]byte(r.URL.Path))
	}))
	t.Cleanup(server.Close)

	cfg := configmock.New(t)
	cfg.Set("metrics_port", serverPort(t, server.URL), pkgconfigmodel.SourceAgentRuntime)

	pdata, err := readProfileData(ipcmock.New(t).GetClient(), 30)
	require.NoError(t, err)

	assert.Equal(t, []byte("/debug/pprof/heap"), pdata["datadog-cluster-agent-1st-heap.pprof"])
	assert.Equal(t, []byte("/debug/pprof/profile"), pdata["datadog-cluster-agent-cpu.pprof"])
	assert.Equal(t, []byte("/debug/pprof/heap"), pdata["datadog-cluster-agent-2nd-heap.pprof"])
	assert.Equal(t, []byte("/debug/pprof/mutex"), pdata["datadog-cluster-agent-mutex.pprof"])
	assert.Equal(t, []byte("/debug/pprof/block"), pdata["datadog-cluster-agent-block.pprof"])
	assert.Equal(t, []byte("/debug/pprof/trace"), pdata["datadog-cluster-agent.trace"])

	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, paths, "/debug/pprof/profile?seconds=30")
	assert.Contains(t, paths, "/debug/pprof/trace?seconds=30")
}
