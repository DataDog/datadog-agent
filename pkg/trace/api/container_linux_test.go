// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package api

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/api/internal/header"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/stretchr/testify/assert"
)

func TestConnContext(t *testing.T) {
	sockPath := "/tmp/test-trace.sock"
	payload := msgpTraces(t, pb.Traces{testutil.RandomTrace(10, 20)})
	client := http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", sockPath)
			},
		},
	}

	fi, err := os.Stat(sockPath)
	if err == nil {
		// already exists
		if fi.Mode()&os.ModeSocket == 0 {
			t.Fatalf("cannot reuse %q; not a unix socket", sockPath)
		}
		if err := os.Remove(sockPath); err != nil {
			t.Fatalf("unable to remove stale socket: %v", err)
		}
	}
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("error listening on unix socket %s: %v", sockPath, err)
	}
	if err := os.Chmod(sockPath, 0o722); err != nil {
		t.Fatalf("error setting socket permissions: %v", err)
	}
	defer ln.Close()

	s := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ucred, ok := r.Context().Value(ucredKey{}).(*syscall.Ucred)
			if !ok || ucred == nil {
				t.Fatalf("Expected a unix credential but found nothing.")
			}
			io.WriteString(w, "OK")
		}),
		ConnContext: connContext,
	}
	go s.Serve(ln)

	resp, err := client.Post("http://localhost:8126/v0.4/traces", "application/msgpack", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected http.StatusOK, got response: %#v", resp)
	}
}

func TestGetContainerID(t *testing.T) {
	const containerID = "abcdef"
	const containerPID = 1234
	// fudge factor to ease testing, if our tests take over 24 hours we got bigger problems
	timeFudgeFactor := 24 * time.Hour
	c := NewCache(timeFudgeFactor)
	c.Store(time.Now().Add(timeFudgeFactor), strconv.Itoa(containerPID), containerID, nil)
	provider := &cgroupIDProvider{
		procRoot:   "",
		controller: "",
		cache:      c,
	}

	t.Run("header", func(t *testing.T) {
		req, err := http.NewRequest("GET", "http://example.com", nil)
		if !assert.NoError(t, err) {
			t.Fail()
		}
		req.Header.Add(header.ContainerID, containerID)
		assert.Equal(t, containerID, provider.GetContainerID(req.Context(), req.Header))
	})

	t.Run("header-cred", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ucredKey{}, &syscall.Ucred{Pid: containerPID})
		req, err := http.NewRequestWithContext(ctx, "GET", "http://example.com", nil)
		if !assert.NoError(t, err) {
			t.Fail()
		}
		req.Header.Add(header.ContainerID, containerID)
		assert.Equal(t, containerID, provider.GetContainerID(req.Context(), req.Header))
	})

	t.Run("cred", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ucredKey{}, &syscall.Ucred{Pid: containerPID})
		req, err := http.NewRequestWithContext(ctx, "GET", "http://example.com", nil)
		if !assert.NoError(t, err) {
			t.Fail()
		}
		assert.Equal(t, containerID, provider.GetContainerID(req.Context(), req.Header))
	})

	t.Run("badcred", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ucredKey{}, &syscall.Ucred{Pid: 2345})
		req, err := http.NewRequestWithContext(ctx, "GET", "http://example.com", nil)
		if !assert.NoError(t, err) {
			t.Fail()
		}
		assert.Equal(t, "", provider.GetContainerID(req.Context(), req.Header))
	})

	t.Run("empty", func(t *testing.T) {
		req, err := http.NewRequest("GET", "http://example.com", nil)
		if !assert.NoError(t, err) {
			t.Fail()
		}
		assert.Equal(t, "", provider.GetContainerID(req.Context(), req.Header))
	})
}
