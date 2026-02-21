// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && linux_bpf

package precompiledserver

import (
	"crypto/tls"
	"fmt"
	nethttp "net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/go/goversion"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
)

// RunHTTPSGoServer starts a Go-based HTTPS server for testing purposes.
// It fetches pre-compiled binaries from the specified directory based on the Go version and architecture.
func RunHTTPSGoServer(tb testing.TB, goVersion goversion.GoVersion, port int) int {
	curDir, err := testutil.CurDir()
	require.NoError(tb, err)

	parentDir := filepath.Dir(curDir)

	cert, key, err := testutil.GetCertsPaths()
	require.NoError(tb, err)

	serverPath := filepath.Join(parentDir, "testdata", "builds", fmt.Sprintf("https-go%s-%s", goVersion.String(), runtime.GOARCH))
	cmd := exec.Command(serverPath, "--cert", cert, "--key", key, "--port", strconv.Itoa(port))
	require.NoError(tb, cmd.Start())
	tb.Cleanup(func() {
		if err := cmd.Process.Kill(); err != nil {
			tb.Logf("Failed to kill process: %v", err)
		}
	})

	// wait for process to be alive
	require.Eventuallyf(tb, func() bool {
		if !isProcessRunning(cmd.Process.Pid) {
			return false
		}

		client := &nethttp.Client{
			Transport: &nethttp.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
		// run health check to ensure the server is up
		resp, err := client.Get(fmt.Sprintf("https://localhost:%d/health", port))
		if err != nil {
			tb.Logf("Health check failed: %v", err)
			return false
		}
		defer resp.Body.Close()
		if resp.StatusCode != nethttp.StatusOK {
			tb.Logf("Health check returned status code %d, expected %d", resp.StatusCode, nethttp.StatusOK)
			return false
		}
		return true
	}, 5*time.Second, 500*time.Millisecond, "Process did not start successfully")

	return cmd.Process.Pid
}

func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Send signal 0 (null signal) to check if process exists
	return process.Signal(syscall.Signal(0)) == nil
}
