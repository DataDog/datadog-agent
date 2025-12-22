// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && linux_bpf

// Package test provides test helpers for the privileged logs module.
package test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/system-probe/modules"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/server"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Handler is a test handler for the privileged logs module.
type Handler struct {
	wg      sync.WaitGroup
	handler http.Handler
	// Called is set to true when the handler has been called.
	Called bool
	// SocketPath is the path to the socket file used to communicate with the
	// privileged logs module.
	SocketPath string
}

// ServeHTTP serves the HTTP requests to the privileged logs module.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Lock due to the setreuid(2) call below.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Get current EUID to restore later
	originalEUID := syscall.Geteuid()

	// Temporarily escalate to root for file access (this thread only).  Need to
	// use the raw system call since the wrapper sets it on all threads.
	if _, _, err := syscall.Syscall(syscall.SYS_SETREUID, ^uintptr(0), 0, 0); err != 0 {
		http.Error(w, fmt.Sprintf("Privilege escalation failed: %v", err),
			http.StatusInternalServerError)
		return
	}

	// Use a wait group to allow the test cleanup to wait for the handler to
	// finish, since Shutdown() on the http server does not wait for hijacked
	// connections (which the privileged logs module uses).
	h.wg.Add(1)

	defer func() {
		if _, _, err := syscall.Syscall(syscall.SYS_SETREUID, ^uintptr(0), uintptr(originalEUID), 0); err != 0 {
			// Log error but can't do much else in test context
			log.Errorf("Failed to restore EUID: %v", err)
		}
		h.wg.Done()
	}()

	// Call the wrapped handler
	h.Called = true
	h.handler.ServeHTTP(w, r)
}

// Setup sets up the test environment for the privileged logs module.
//
// The tests are run as root since they're part of the system-probe tests.  This
// allows us to test the privileged logs module by keeping the log files
// inaccessible to the logs tailing code but accessible to the privileged logs
// module.  We do this by changing the EUID to an unprivileged user here but
// changing it back to root in the privileged logs test server, and by creating
// all log files with 000 permissions which only EUID root can override.
func Setup(t *testing.T, callback func()) *Handler {
	unprivilegedUID := 0
	sudoUID := os.Getenv("SUDO_UID")
	if sudoUID != "" {
		var err error
		unprivilegedUID, err = strconv.Atoi(sudoUID)
		require.NoError(t, err)
	}

	if unprivilegedUID == 0 {
		user, err := user.Lookup("nobody")
		require.NoError(t, err)
		unprivilegedUID, err = strconv.Atoi(user.Uid)
		require.NoError(t, err)
	}

	err := syscall.Seteuid(unprivilegedUID)
	require.NoError(t, err)
	t.Cleanup(func() {
		err := syscall.Seteuid(0)
		require.NoError(t, err)
	})

	// Set up privileged-logs server
	handler := setupTestServer(t)

	// Operations such as creating temp directories need to be done after the
	// user change but before the umask change.
	if callback != nil {
		callback()
	}

	// Set umask so all created files have 000 permissions and are not readable
	// by the unprivileged user
	oldUmask := syscall.Umask(0777)
	t.Cleanup(func() {
		syscall.Umask(oldUmask)
	})

	return handler
}

// WithParentPermFixup ensures that the parent directory of the given path has
// execute permissions before operations that manipulate the log files from the
// tests.  This is necessary to ensure that the tests can manipulate the log
// files even when the parent directory does not have search permissions.
func WithParentPermFixup(t *testing.T, path string, op func() error) error {
	parent := filepath.Dir(path)
	err := os.Chmod(parent, 0755)
	require.NoError(t, err)

	opErr := op()

	err = os.Chmod(parent, 0)
	require.NoError(t, err)

	t.Cleanup(func() {
		err := os.Chmod(parent, 0755)
		require.NoError(t, err)
	})

	return opErr
}

func setupTestServer(t *testing.T) *Handler {
	cfg := &sysconfigtypes.Config{}
	deps := module.FactoryDependencies{}

	fdModule, err := modules.PrivilegedLogs.Fn(cfg, deps)
	if err != nil {
		t.Fatalf("Failed to create privileged logs module: %v", err)
	}

	// Use /tmp for shorter socket paths to avoid Unix socket limits
	tempDir, err := os.MkdirTemp("/tmp", "fdtest")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	socketPath := filepath.Join(tempDir, "fd.sock")
	listener, err := server.NewListener(socketPath)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to create listener: %v", err)
	}

	// Set up HTTP router and register the module
	httpMux := mux.NewRouter()
	router := module.NewRouter("privileged_logs", httpMux)
	err = fdModule.Register(router)
	if err != nil {
		listener.Close()
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to register module: %v", err)
	}

	testHandler := &Handler{
		handler:    httpMux,
		SocketPath: socketPath,
	}
	httpServer := &http.Server{
		Handler: testHandler,
	}

	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			t.Logf("Server error: %v", err)
		}
	}()

	// Wait for server to be ready
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		conn, err := net.Dial("unix", socketPath)
		require.NoError(collect, err)
		conn.Close()
	}, 1*time.Second, 10*time.Millisecond)

	t.Cleanup(func() {
		require.NoError(t, httpServer.Shutdown(context.Background()))
		testHandler.wg.Wait()
		listener.Close()
		os.RemoveAll(tempDir)
	})

	return testHandler
}
