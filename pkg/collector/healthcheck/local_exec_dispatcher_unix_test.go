// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin

package healthcheck

import (
	"context"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
)

func TestLocalExecDispatcherOutputLimit(t *testing.T) {
	for _, redirect := range []string{"", " >&2", " 2>&1"} {
		t.Run("redirect="+redirect, func(t *testing.T) {
			dispatcher := NewLocalExecDispatcher(nil, "test-host")
			command := "i=0; while [ \"$i\" -lt 10000 ]; do printf x; printf x >&2; i=$((i+1)); done"
			exitCode, output, err := dispatcher.run(context.Background(), "("+command+")"+redirect)
			require.NoError(t, err)
			require.Zero(t, exitCode)
			require.Equal(t, strings.Repeat("x", localOutputLimit), output)
		})
	}
}

func TestLocalExecObserverShutdownKillsChildren(t *testing.T) {
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	require.NoError(t, listener.SetDeadline(time.Now().Add(10*time.Second)))
	executable, err := os.Executable()
	require.NoError(t, err)
	cfg := healthConfig()
	cfg.Remediation.Steps[0].Command = "DD_HEALTHCHECK_CHILD_ADDRESS='" + listener.Addr().String() + "' '" + strings.ReplaceAll(executable, "'", "'\"'\"'") + "' -test.run=^TestLocalExecChildProcess$ & wait"
	id := registerTestCheck(t, cfg)
	observer := NewObserver(NewLocalExecDispatcher(nil, "test-host"))
	t.Cleanup(observer.Stop)
	observer.ObserveServiceCheck(id, cfg.ServiceCheck, servicecheck.ServiceCheckOK, "", "", nil)
	observer.ObserveServiceCheck(id, cfg.ServiceCheck, servicecheck.ServiceCheckCritical, "unhealthy", "", nil)
	child, err := listener.Accept()
	require.NoError(t, err, "child must start before observer shutdown")
	t.Cleanup(func() { _ = child.Close() })

	stopped := make(chan struct{})
	go func() {
		observer.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(localWaitDelay + 5*time.Second):
		t.Fatal("observer did not stop after cancelling local execution")
	}
	require.NoError(t, child.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, err = child.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF, "shutdown must terminate the child, closing its connection")
}

func TestLocalExecChildProcess(t *testing.T) {
	address := os.Getenv("DD_HEALTHCHECK_CHILD_ADDRESS")
	if address == "" {
		t.Skip("subprocess helper")
	}
	conn, err := net.Dial("tcp", address)
	require.NoError(t, err)
	defer conn.Close()
	_, _ = conn.Read(make([]byte, 1))
}
