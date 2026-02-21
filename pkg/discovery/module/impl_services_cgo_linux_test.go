// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Parity tests for the CGO-backed getServices path.  They cover behaviours
// that are implemented in the Go wrapper rather than inside the Rust library:
//
//   - service_type must be non-empty (computed by servicetype.Detect on the Go
//     side; the Rust library always sets it to an empty string).
//   - Processes in IgnoreComms must be absent from Services even when the Rust
//     library returns them (comm-name filtering lives on the Go side).

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

// TestCGOServiceTypePopulated verifies that the Go wrapper populates
// service_type via servicetype.Detect.  The Rust library always returns an
// empty string for this field; if the wrapper does not call Detect the
// returned Type would be "", breaking callers that rely on it.
func TestCGOServiceTypePopulated(t *testing.T) {
	disc := setupDiscoveryModule(t)

	// Give the child process a listening socket so discovery includes it.
	serverf, _ := startTCPServer(t, "tcp4", "")
	cmd := startProcessWithFile(t, serverf)
	pid := cmd.Process.Pid

	location := disc.url + "/" + string(config.DiscoveryModule) + pathServices

	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		params := &core.Params{NewPids: getRunningPids(collect)}
		resp := makeRequest[model.ServicesResponse](collect, location, params)
		svc := findService(pid, resp.Services)
		require.NotNilf(collect, svc, "service for pid %d not found", pid)
		// Any non-empty string proves that servicetype.Detect was invoked on the
		// Go side.  An ephemeral port maps to "web_service" by default.
		assert.NotEmpty(collect, svc.Type,
			"service_type must be non-empty: the Go wrapper must call servicetype.Detect "+
				"because the Rust library always returns an empty string")
	}, 30*time.Second, 100*time.Millisecond)
}

// TestCGOIgnoreCommFiltering verifies that comm-name filtering is applied by
// the Go wrapper.  The Rust library has no equivalent of the Go-side
// ignored_command_names configuration.
//
// Strategy: create the module with "sleep" pre-added to IgnoreComms; start a
// "sleep" process that holds a listening socket (so it would normally be
// reported); confirm it never appears in Services during a polling window.
func TestCGOIgnoreCommFiltering(t *testing.T) {
	mod, err := NewDiscoveryModule(nil, module.FactoryDependencies{})
	require.NoError(t, err)
	d := mod.(*discovery)
	// Set IgnoreComms before the first request to avoid data races.
	d.config.IgnoreComms = map[string]struct{}{"sleep": {}}

	mux := gorillamux.NewRouter()
	d.Register(module.NewRouter(string(config.DiscoveryModule), mux))
	t.Cleanup(d.Close)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Give the child process a TCP listening socket so it would be visible to
	// discovery in the absence of comm-name filtering.
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

	// Poll for several seconds: the process must never appear in Services.
	// We allow enough time for the process to fully start up.
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
