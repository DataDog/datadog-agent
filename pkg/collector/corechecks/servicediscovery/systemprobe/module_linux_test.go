// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package systemprobe

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"testing"

	gorillamux "github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	workloadmetacomp "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/stretchr/testify/require"
)

func setupServiceDiscoveryModule(t *testing.T) string {
	t.Helper()

	wmeta := optional.NewNoneOption[workloadmetacomp.Component]()
	mux := gorillamux.NewRouter()
	cfg := &types.Config{
		Enabled: true,
		EnabledModules: map[types.ModuleName]struct{}{
			config.ServiceDiscoveryModule: {},
		},
	}
	err := module.Register(cfg, mux, []module.Factory{ServiceDiscoveryModule}, wmeta, nil)
	require.NoError(t, err)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func startServerAndGetPort(t *testing.T, modURL string) *model.Port {
	t.Helper()

	// start a process listening at some port
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ln.Close()
	})
	addr := ln.Addr().(*net.TCPAddr)

	req, err := http.NewRequest("GET", modURL+"/service_discovery/open_ports", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	res := &model.OpenPortsResponse{}
	err = json.NewDecoder(resp.Body).Decode(res)
	require.NoError(t, err)
	require.NotEmpty(t, res)

	for _, p := range res.Ports {
		if p.Port == addr.Port {
			return p
		}
	}
	return nil
}

func TestServiceDiscoveryModule_OpenPorts(t *testing.T) {
	url := setupServiceDiscoveryModule(t)

	port := startServerAndGetPort(t, url)
	require.NotNil(t, port, "could not find http server port")
	assert.Equal(t, "tcp", port.Proto)

	// should be able to get this info since it's a child process, and it will be owned by the current user
	assert.NotEmpty(t, port.ProcessName)
	assert.NotEmpty(t, port.PID)
}

func TestServiceDiscoveryModule_GetProc(t *testing.T) {
	url := setupServiceDiscoveryModule(t)
	port := startServerAndGetPort(t, url)
	require.NotNil(t, port, "could not find http server port")
	require.NotEmpty(t, port.PID, "could not get port pid")

	req, err := http.NewRequest("GET", url+"/service_discovery/procs/"+strconv.Itoa(port.PID), nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	res := &model.GetProcResponse{}
	err = json.NewDecoder(resp.Body).Decode(res)
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, res.Proc.PID, port.PID)
	assert.NotEmpty(t, res.Proc.Environ)
	assert.NotEmpty(t, res.Proc.CWD)
}
