// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package kubeinfo

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/version"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// syncBuffer is a bytes.Buffer safe for concurrent use by the logger and the test.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func captureLogs(t *testing.T) *syncBuffer {
	var logs syncBuffer
	logger, err := log.LoggerFromWriterWithMinLevelAndLvlMsgFormat(&logs, log.InfoLvl)
	require.NoError(t, err)
	log.SetupLogger(logger, "info")
	t.Cleanup(func() { log.SetupLogger(log.Default(), "off") })
	return &logs
}

func TestWarnOnUnsupportedVersion(t *testing.T) {
	tests := []struct {
		name          string
		serverVersion *version.Info
		setBefore     bool
		wantWarning   bool
	}{
		{name: "unsupported, collected before", serverVersion: &version.Info{GitVersion: "v1.22.17"}, setBefore: true, wantWarning: true},
		{name: "unsupported, collected concurrently", serverVersion: &version.Info{GitVersion: "v1.22.17"}, wantWarning: true},
		{name: "supported", serverVersion: &version.Info{GitVersion: "v1.30.2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wmeta := newWorkloadmeta(t)
			// After newWorkloadmeta, which sets up its own logger.
			logs := captureLogs(t)
			if tt.setBefore {
				wmeta.Set(kubeCapabilities(tt.serverVersion))
			}

			done := make(chan struct{})
			go func() {
				defer close(done)
				WarnOnUnsupportedVersion(context.Background(), wmeta)
			}()
			if !tt.setBefore {
				wmeta.Set(kubeCapabilities(tt.serverVersion))
			}

			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("WarnOnUnsupportedVersion did not return after the version was collected")
			}
			log.Flush()

			warnings := strings.Count(logs.String(), "[DEPRECATION WARNING]")
			if tt.wantWarning {
				assert.Equal(t, 1, warnings)
				assert.Contains(t, logs.String(), "Kubernetes v1.22.17 is older than v1.23")
			} else {
				assert.Zero(t, warnings)
			}
		})
	}
}

func TestWarnOnUnsupportedVersionStopsOnContextCancel(t *testing.T) {
	wmeta := newWorkloadmeta(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		WarnOnUnsupportedVersion(ctx, wmeta)
	}()
	// Covers both canceling before and while waiting for the version.
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("WarnOnUnsupportedVersion did not return after the context was canceled")
	}
}

func TestVersionStatus(t *testing.T) {
	tests := []struct {
		name            string
		serverVersion   *version.Info
		wantDetected    string
		wantUnsupported bool
	}{
		{name: "nil"},
		{name: "supported", serverVersion: &version.Info{GitVersion: "v1.23.0"}, wantDetected: "v1.23.0"},
		{name: "unsupported", serverVersion: &version.Info{GitVersion: "v1.22.17-eks-a1b2c3"}, wantDetected: "v1.22.17-eks-a1b2c3", wantUnsupported: true},
		{name: "major minor only", serverVersion: &version.Info{Major: "1", Minor: "22+"}, wantDetected: "v1.22+", wantUnsupported: true},
		{name: "unparsable", serverVersion: &version.Info{GitVersion: "foo"}, wantDetected: "foo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detected, unsupported := versionStatus(tt.serverVersion)
			assert.Equal(t, tt.wantDetected, detected)
			assert.Equal(t, tt.wantUnsupported, unsupported)
		})
	}
}
