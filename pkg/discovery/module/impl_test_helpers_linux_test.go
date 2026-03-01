// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// This doesn't need BPF, but it's built with this tag to only run with
// system-probe tests.
//go:build test && linux_bpf

package module

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	gorillamux "github.com/gorilla/mux"
	"github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/discovery/core"
	"github.com/DataDog/datadog-agent/pkg/discovery/model"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	usmtestutil "github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
)

type testDiscoveryModule struct {
	url string
}

func setupDiscoveryModule(t *testing.T) *testDiscoveryModule {
	t.Helper()
	mux := gorillamux.NewRouter()

	mod, err := NewDiscoveryModule(nil, module.FactoryDependencies{})
	require.NoError(t, err)
	discovery := mod.(*discovery)

	discovery.Register(module.NewRouter(string(config.DiscoveryModule), mux))
	t.Cleanup(discovery.Close)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &testDiscoveryModule{
		url: srv.URL,
	}
}

// makeRequest wraps the request to the discovery module, setting the JSON body if provided,
// and returning the response as the given type.
func makeRequest[T any](t require.TestingT, url string, params *core.Params) *T {
	var body *bytes.Buffer
	if params != nil {
		jsonData, err := params.ToJSON()
		require.NoError(t, err, "failed to serialize params to JSON")
		body = bytes.NewBuffer(jsonData)
	}

	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequest(http.MethodPost, url, body)
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequest(http.MethodPost, url, nil)
	}
	require.NoError(t, err, "failed to create request")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "failed to send request")
	defer resp.Body.Close()

	res := new(T)
	err = json.NewDecoder(resp.Body).Decode(res)
	require.NoError(t, err, "failed to decode response")

	return res
}

// getRunningPids wraps the process.Pids function, returning a slice of ints
// that can be used as the pids query param.
func getRunningPids(t require.TestingT) []int32 {
	pids, err := process.Pids()
	require.NoError(t, err)
	return pids
}

// getServices calls the /discovery/services endpoint using all running PIDs.
func getServices(t require.TestingT, url string) *model.ServicesResponse {
	location := url + "/" + string(config.DiscoveryModule) + pathServices
	params := &core.Params{
		NewPids: getRunningPids(t),
	}
	return makeRequest[model.ServicesResponse](t, location, params)
}

func newDiscovery() *discovery {
	mod, err := NewDiscoveryModule(nil, module.FactoryDependencies{})
	if err != nil {
		panic(err)
	}
	return mod.(*discovery)
}

func makeAlias(t *testing.T, alias string, serverBin string) string {
	binDir := filepath.Dir(serverBin)
	aliasPath := filepath.Join(binDir, alias)

	target, err := os.Readlink(aliasPath)
	if err == nil && target == serverBin {
		return aliasPath
	}

	os.Remove(aliasPath)
	err = os.Symlink(serverBin, aliasPath)
	require.NoError(t, err)

	return aliasPath
}

func buildFakeServer(t *testing.T) string {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)
	serverBin, err := usmtestutil.BuildGoBinaryWrapper(filepath.Join(curDir, "testutil"), "fake_server")
	require.NoError(t, err)

	for _, alias := range []string{"java", "node", "sshd", "dotnet"} {
		makeAlias(t, alias, serverBin)
	}

	return filepath.Dir(serverBin)
}
