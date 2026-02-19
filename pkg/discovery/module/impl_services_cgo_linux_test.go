// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Parity tests for the CGO-backed getServices path.  They cover behaviours
// that are implemented in the Go wrapper rather than inside the Rust library:
//
//   - service_type must be non-empty (computed by servicetype.Detect on the Go
//     side; the Rust library always sets it to an empty string).
//   - Processes in IgnoreComms must be absent from Services: NewPids are
//     post-filtered after the Rust library returns; HeartbeatPids are
//     pre-filtered before being passed to the Rust library.

//go:build test && linux_bpf && cgo

package module

import (
	"context"
	"net"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"

	gorillamux "github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/discovery/core"
	"github.com/DataDog/datadog-agent/pkg/discovery/model"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
)

// TestCGOServiceTypePopulated verifies that service_type is populated by the
// Go wrapper via servicetype.Detect. The Rust library always returns an empty
// string for this field.
func TestCGOServiceTypePopulated(t *testing.T) {
	disc := setupDiscoveryModule(t)

	serverf, _ := startTCPServer(t, "tcp4", "")
	cmd := startProcessWithFile(t, serverf)
	pid := cmd.Process.Pid

	location := disc.url + "/" + string(config.DiscoveryModule) + pathServices

	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		params := &core.Params{NewPids: getRunningPids(collect)}
		resp := makeRequest[model.ServicesResponse](collect, location, params)
		svc := findService(pid, resp.Services)
		require.NotNilf(collect, svc, "service for pid %d not found", pid)
		assert.NotEmpty(collect, svc.Type)
	}, 30*time.Second, 100*time.Millisecond)
}

// TestCGOIgnoreCommFiltering verifies that NewPids in IgnoreComms are filtered
// from Services by the Go wrapper after the Rust library returns.
func TestCGOIgnoreCommFiltering(t *testing.T) {
	mod, err := NewDiscoveryModule(nil, module.FactoryDependencies{})
	require.NoError(t, err)
	d := mod.(*discovery)
	d.config.IgnoreComms = map[string]struct{}{"sleep": {}}

	mux := gorillamux.NewRouter()
	d.Register(module.NewRouter(string(config.DiscoveryModule), mux))
	t.Cleanup(d.Close)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	listener, err := net.Listen("tcp", "")
	require.NoError(t, err)
	f, err := listener.(*net.TCPListener).File()
	listener.Close()
	require.NoError(t, err)
	disableCloseOnExec(t, f)

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "sleep", "1000")
	require.NoError(t, cmd.Start())
	f.Close()
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})

	pid := cmd.Process.Pid
	location := srv.URL + "/" + string(config.DiscoveryModule) + pathServices

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		params := &core.Params{NewPids: getRunningPids(t)}
		resp := makeRequest[model.ServicesResponse](t, location, params)
		if svc := findService(pid, resp.Services); svc != nil {
			t.Fatalf("pid %d ('sleep') appeared in Services despite being in IgnoreComms", pid)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// TestCGOHeartbeatIgnoreCommFiltering verifies that HeartbeatPids in IgnoreComms
// are pre-filtered by the Go wrapper before being passed to the Rust library.
func TestCGOHeartbeatIgnoreCommFiltering(t *testing.T) {
	mod, err := NewDiscoveryModule(nil, module.FactoryDependencies{})
	require.NoError(t, err)
	d := mod.(*discovery)
	d.config.IgnoreComms = map[string]struct{}{"sleep": {}}

	mux := gorillamux.NewRouter()
	d.Register(module.NewRouter(string(config.DiscoveryModule), mux))
	t.Cleanup(d.Close)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	listener, err := net.Listen("tcp", "")
	require.NoError(t, err)
	f, err := listener.(*net.TCPListener).File()
	listener.Close()
	require.NoError(t, err)
	disableCloseOnExec(t, f)

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "sleep", "1000")
	require.NoError(t, cmd.Start())
	f.Close()
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})

	pid := cmd.Process.Pid
	location := srv.URL + "/" + string(config.DiscoveryModule) + pathServices

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		params := &core.Params{HeartbeatPids: []int32{int32(pid)}}
		resp := makeRequest[model.ServicesResponse](t, location, params)
		if svc := findService(pid, resp.Services); svc != nil {
			t.Fatalf("pid %d ('sleep') appeared in Services via HeartbeatPids despite being in IgnoreComms", pid)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
