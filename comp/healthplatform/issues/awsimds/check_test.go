// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package awsimds

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withIMDSAddress points the check at a local listener for the duration of fn,
// restoring the real IMDS address afterwards.
func withIMDSAddress(t *testing.T, addr string, fn func()) {
	t.Helper()
	original := imdsAddress
	imdsAddress = addr
	defer func() { imdsAddress = original }()
	fn()
}

// withContainerMarker sets DOCKER_DD_AGENT, which env.IsContainerized() checks -
// it's baked into the official Agent Dockerfiles, so it's set regardless of which
// container runtime (Docker, containerd, CRI-O) actually runs the image.
func withContainerMarker(t *testing.T, fn func()) {
	t.Helper()
	t.Setenv("DOCKER_DD_AGENT", "true")
	fn()
}

// TestCheck_HopLimitTooLow reproduces the real-world symptom observed on AWS: the TCP
// handshake to the IMDS endpoint always succeeds (SYN-ACK is not subject to
// HttpPutResponseHopLimit), but the token PUT response is dropped in transit, so
// reading it hangs until the client's timeout fires. A listener that accepts
// connections but never writes a response simulates this precisely.
func TestCheck_HopLimitTooLow(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// Accept the connection and read the request, but never respond -
			// this is what a dropped-in-transit token response looks like.
			go func() {
				defer conn.Close()
				buf := make([]byte, 1024)
				_, _ = conn.Read(buf)
				time.Sleep(5 * time.Second)
			}()
		}
	}()

	withContainerMarker(t, func() {
		withIMDSAddress(t, ln.Addr().String(), func() {
			original := requestTimeout
			requestTimeout = 200 * time.Millisecond
			defer func() { requestTimeout = original }()

			issues, err := Check()
			require.NoError(t, err)
			require.Len(t, issues, 1)
			assert.Equal(t, IssueID, issues[0].IssueID)
		})
	})
}

// TestCheck_IMDSReachable verifies that a normally-responding IMDS produces no issue,
// regardless of the response status code.
func TestCheck_IMDSReachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/latest/api/token", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fake-token"))
	}))
	defer srv.Close()

	withContainerMarker(t, func() {
		withIMDSAddress(t, srv.Listener.Addr().String(), func() {
			issues, err := Check()
			require.NoError(t, err)
			assert.Empty(t, issues)
		})
	})
}

// TestCheck_NotContainerized verifies the check is a no-op outside of containers,
// even if the (fake) IMDS endpoint would otherwise time out.
func TestCheck_NotContainerized(t *testing.T) {
	t.Setenv("DOCKER_DD_AGENT", "")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	withIMDSAddress(t, ln.Addr().String(), func() {
		issues, err := Check()
		require.NoError(t, err)
		assert.Empty(t, issues)
	})
}
