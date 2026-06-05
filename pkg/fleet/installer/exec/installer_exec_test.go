// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package exec

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

type telemetryCaptureClient struct {
	payloads chan []byte
}

func (c *telemetryCaptureClient) Do(req *http.Request) (*http.Response, error) {
	payload, err := io.ReadAll(req.Body)
	if err == nil {
		c.payloads <- payload
	}
	return &http.Response{
		StatusCode: http.StatusAccepted,
		Status:     "202 Accepted",
		Body:       io.NopCloser(strings.NewReader("")),
	}, err
}

func TestGarbageCollectContextCanceledDoesNotMarkSpanError(t *testing.T) {
	helperPath := filepath.Join(t.TempDir(), "installer-helper")
	readyPath := filepath.Join(t.TempDir(), "ready")
	require.NoError(t, os.WriteFile(helperPath, []byte("#!/bin/sh\ntouch \"$INSTALLER_EXEC_READY_FILE\"\nexec sleep 1000\n"), 0o755))
	t.Setenv("INSTALLER_EXEC_READY_FILE", readyPath)

	client := &telemetryCaptureClient{payloads: make(chan []byte, 4)}
	telem := telemetry.NewTelemetry(client, "api-key", "datadoghq.com", "test-service")
	stopped := false

	ctx, cancel := context.WithCancel(telemetry.WithSamplingPriority(context.Background(), 2))
	t.Cleanup(func() {
		cancel()
		if !stopped {
			telem.Stop()
		}
	})

	installer := NewInstallerExec(&env.Env{}, helperPath)
	errCh := make(chan error, 1)
	go func() {
		errCh <- installer.GarbageCollect(ctx)
	}()

	require.Eventually(t, func() bool {
		_, err := os.Stat(readyPath)
		return err == nil
	}, time.Second, 10*time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("garbage-collect command did not exit after context cancellation")
	}

	telem.Stop()
	stopped = true

	var payload []byte
	select {
	case payload = <-client.payloads:
	default:
		t.Fatal("expected a telemetry payload")
	}

	var event struct {
		RequestType string `json:"request_type"`
		Payload     struct {
			Traces [][]struct {
				Name  string            `json:"name"`
				Error int32             `json:"error"`
				Meta  map[string]string `json:"meta"`
			} `json:"traces"`
		} `json:"payload"`
	}
	require.NoError(t, json.Unmarshal(payload, &event))
	assert.Equal(t, "traces", event.RequestType)

	found := false
	for _, trace := range event.Payload.Traces {
		for _, span := range trace {
			if span.Name != "installer.garbage-collect" {
				continue
			}
			found = true
			assert.Equal(t, int32(0), span.Error)
			assert.NotContains(t, span.Meta, "error.message")
		}
	}
	assert.True(t, found, "expected installer.garbage-collect span")
}
