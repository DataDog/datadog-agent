// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package systemprobe

import (
	"encoding/json"
	"fmt"
	"golang.org/x/sys/unix"
	"net"
	"net/http"
	"os"
	"slices"
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
	m := module.Factory{
		Name:             config.ServiceDiscoveryModule,
		ConfigNamespaces: []string{"service_discovery"},
		Fn:               NewServiceDiscoveryModule,
		NeedsEBPF: func() bool {
			return false
		},
	}
	err := module.Register(cfg, mux, []module.Factory{m}, wmeta, nil)
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

func Test_getInternalEnvs(t *testing.T) {
	// get the pid of the current process
	curPid := os.Getpid()
	// get the envs
	envs := getInternalEnvs(curPid)
	// should be nil
	if envs != nil {
		t.Error("should not have any envs found")
	}
	// write a memory mapped file
	_, err := memfile("envs", []byte("key1=val1\nkey2=val2\nkey3=val3\n"))
	if err != nil {
		t.Fatalf("error writing memfd file: %v", err)
	}
	// get the envs
	envs = getInternalEnvs(curPid)
	// should be non-nil
	expected := []string{"key1=val1", "key2=val2", "key3=val3"}
	if !slices.Equal(envs, expected) {
		t.Errorf("expected %v, got %v", expected, envs)
	}
}

// memfile takes a file name used, and the byte slice
// containing data the file should contain.
//
// name does not need to be unique, as it's used only
// for debugging purposes.
//
// It is up to the caller to close the returned descriptor.
func memfile(name string, b []byte) (int, error) {
	fd, err := unix.MemfdCreate(name, 0)
	if err != nil {
		return 0, fmt.Errorf("MemfdCreate: %v", err)
	}

	err = unix.Ftruncate(fd, int64(len(b)))
	if err != nil {
		return 0, fmt.Errorf("Ftruncate: %v", err)
	}

	data, err := unix.Mmap(fd, 0, len(b), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return 0, fmt.Errorf("Mmap: %v", err)
	}

	copy(data, b)

	err = unix.Munmap(data)
	if err != nil {
		return 0, fmt.Errorf("Munmap: %v", err)
	}

	return fd, nil
}
