// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package configingestion

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockStore implements the workloadmeta.Component methods used by Watcher.
// Unused methods are satisfied by embedding the interface; they panic if called.
type mockStore struct {
	workloadmeta.Component
	ch chan workloadmeta.EventBundle
}

func (m *mockStore) Subscribe(_ string, _ workloadmeta.SubscriberPriority, _ *workloadmeta.Filter) chan workloadmeta.EventBundle {
	return m.ch
}

func (m *mockStore) Unsubscribe(_ chan workloadmeta.EventBundle) {}

// newTestWatcher creates a Watcher pointing at srv (or with an empty URL if srv
// is nil). The store uses a nil channel so Start must not be called; for
// lifecycle tests use newTestWatcherWithStore instead.
func newTestWatcher(t *testing.T, srv *httptest.Server) *Watcher {
	t.Helper()
	url := ""
	if srv != nil {
		url = srv.URL
	}
	store := &mockStore{ch: nil}
	return NewWatcher(store, Config{IntakeURL: url, APIKey: "test-key", HostID: "test-host"})
}

// newTestWatcherWithStore creates a Watcher backed by a real mockStore channel,
// suitable for tests that call Start/Stop.
func newTestWatcherWithStore(t *testing.T) (*Watcher, chan workloadmeta.EventBundle) {
	t.Helper()
	ch := make(chan workloadmeta.EventBundle)
	store := &mockStore{ch: ch}
	w := NewWatcher(store, Config{IntakeURL: "http://localhost", APIKey: "k", HostID: "h"})
	return w, ch
}

func makeTempConfig(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "redis*.conf")
	require.NoError(t, err)
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

// --- Start/Stop lifecycle ---

func TestWatcher_StartStop_GoroutineExits(t *testing.T) {
	w, _ := newTestWatcherWithStore(t)
	// ch is intentionally not closed or sent on; Stop() cancels the context
	// which drives goroutine exit via the ctx.Done() select arm.
	w.Start(context.Background())

	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()

	select {
	case <-done:
		// goroutine exited cleanly — no leak
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return within 2 seconds — goroutine leak suspected")
	}
}

// --- handleBundle ---

func TestHandleBundle_NewPathIsShipped(t *testing.T) {
	var shipped atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		shipped.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	path := makeTempConfig(t, "bind 127.0.0.1\n")
	w := newTestWatcher(t, srv)

	w.handleBundle(context.Background(), workloadmeta.EventBundle{
		Events: []workloadmeta.Event{
			{
				Type: workloadmeta.EventTypeSet,
				Entity: &workloadmeta.Process{
					Pid: 1,
					Service: &workloadmeta.Service{
						GeneratedName: "redis-server",
						ConfigFiles:   []string{path},
					},
				},
			},
		},
	})

	assert.Equal(t, int32(1), shipped.Load())
	// handleBundle returns synchronously, so no lock needed to inspect maps.
	assert.Contains(t, w.seenFiles, path)
	assert.Contains(t, w.pidFiles[1], path)
}

func TestHandleBundle_AlreadySeenPathIsSkipped(t *testing.T) {
	var shipped atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		shipped.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	path := makeTempConfig(t, "bind 127.0.0.1\n")
	w := newTestWatcher(t, srv)
	w.seenFiles[path] = struct{}{}

	w.handleBundle(context.Background(), workloadmeta.EventBundle{
		Events: []workloadmeta.Event{
			{
				Type: workloadmeta.EventTypeSet,
				Entity: &workloadmeta.Process{
					Pid: 2,
					Service: &workloadmeta.Service{
						GeneratedName: "redis-server",
						ConfigFiles:   []string{path},
					},
				},
			},
		},
	})

	assert.Equal(t, int32(0), shipped.Load())
}

func TestHandleBundle_SamePathInSameBundleShippedOnce(t *testing.T) {
	var shipped atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		shipped.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	path := makeTempConfig(t, "bind 127.0.0.1\n")
	w := newTestWatcher(t, srv)

	// Two Set events in the same bundle reference the same path (different PIDs).
	// The path must be shipped exactly once.
	w.handleBundle(context.Background(), workloadmeta.EventBundle{
		Events: []workloadmeta.Event{
			{
				Type: workloadmeta.EventTypeSet,
				Entity: &workloadmeta.Process{
					Pid:     10,
					Service: &workloadmeta.Service{GeneratedName: "redis-server", ConfigFiles: []string{path}},
				},
			},
			{
				Type: workloadmeta.EventTypeSet,
				Entity: &workloadmeta.Process{
					Pid:     11,
					Service: &workloadmeta.Service{GeneratedName: "redis-server", ConfigFiles: []string{path}},
				},
			},
		},
	})

	assert.Equal(t, int32(1), shipped.Load(), "same path in one bundle must be shipped only once")
	assert.Contains(t, w.seenFiles, path)
}

func TestHandleBundle_NilServiceIsSkipped(t *testing.T) {
	w := newTestWatcher(t, nil)
	// Must not panic when Service is nil.
	w.handleBundle(context.Background(), workloadmeta.EventBundle{
		Events: []workloadmeta.Event{
			{Type: workloadmeta.EventTypeSet, Entity: &workloadmeta.Process{Pid: 5, Service: nil}},
		},
	})
	assert.Empty(t, w.seenFiles)
}

func TestHandleBundle_UnsetRemovesPidFilesNotSeenFiles(t *testing.T) {
	path := makeTempConfig(t, "bind 127.0.0.1\n")
	w := newTestWatcher(t, nil)
	w.seenFiles[path] = struct{}{}
	w.pidFiles[3] = []string{path}

	w.handleBundle(context.Background(), workloadmeta.EventBundle{
		Events: []workloadmeta.Event{
			{
				Type: workloadmeta.EventTypeUnset,
				Entity: &workloadmeta.Process{
					Pid:     3,
					Service: &workloadmeta.Service{GeneratedName: "redis-server", ConfigFiles: []string{path}},
				},
			},
		},
	})

	assert.NotContains(t, w.pidFiles, int32(3))
	assert.Contains(t, w.seenFiles, path)
}

// --- ship ---

func TestShip_NonExistentFileReturnsError(t *testing.T) {
	w := newTestWatcher(t, nil)
	err := w.ship(context.Background(), "/nonexistent/does-not-exist.conf", "redis")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open")
}

func TestShip_BinaryFileRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binary.bin")
	data := make([]byte, 128)
	for i := range data {
		data[i] = 0xFF
	}
	require.NoError(t, os.WriteFile(path, data, 0600))

	w := newTestWatcher(t, nil)
	err := w.ship(context.Background(), path, "redis")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "UTF-8")
}

func TestShip_Non2xxResponseIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	path := makeTempConfig(t, "bind 127.0.0.1\n")
	w := newTestWatcher(t, srv)
	err := w.ship(context.Background(), path, "redis")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestShip_OversizedFileRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.conf")
	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("a", maxFileBytes+1)), 0600))

	w := newTestWatcher(t, nil)
	err := w.ship(context.Background(), path, "redis")
	require.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprintf("%d KiB", maxFileBytes/1024))
}

// --- integrationFromService ---

func TestIntegrationFromService_KnownNames(t *testing.T) {
	cases := []struct {
		generatedName string
		want          string
	}{
		{"redis-server", "redis"},
		{"Redis-Server", "redis"},
		{"mysqld", "mysql"},
		{"postgres", "postgresql"},
		{"mongod", "mongodb"},
		{"nginx", "nginx"},
	}
	for _, tc := range cases {
		t.Run(tc.generatedName, func(t *testing.T) {
			assert.Equal(t, tc.want, integrationFromService(&workloadmeta.Service{GeneratedName: tc.generatedName}))
		})
	}
}

func TestIntegrationFromService_UnknownName(t *testing.T) {
	assert.Equal(t, "myapp", integrationFromService(&workloadmeta.Service{GeneratedName: "MyApp"}))
}

func TestIntegrationFromService_Nil(t *testing.T) {
	assert.Equal(t, "unknown", integrationFromService(nil))
}

// --- redactSensitive ---

func TestRedactSensitive_RequirepassRedacted(t *testing.T) {
	out := redactSensitive("requirepass supersecret\nbind 127.0.0.1\n")
	assert.Contains(t, out, "[REDACTED]")
	assert.NotContains(t, out, "supersecret")
	assert.Contains(t, out, "bind 127.0.0.1")
}

func TestRedactSensitive_YAMLPasswordRedacted(t *testing.T) {
	out := redactSensitive("password: mypass123\nhost: localhost\n")
	assert.Contains(t, out, "[REDACTED]")
	assert.NotContains(t, out, "mypass123")
	assert.Contains(t, out, "host: localhost")
}

func TestRedactSensitive_NoSensitiveKeys(t *testing.T) {
	input := "bind 127.0.0.1\nmaxmemory 100mb\n"
	assert.Equal(t, input, redactSensitive(input))
}

func TestRedactSensitive_WordBoundaryPreventsFalsePositives(t *testing.T) {
	// Keys that merely contain a sensitive substring must not be redacted.
	input := "notapassword: value\nmy_api_key_length: 32\n"
	out := redactSensitive(input)
	assert.Equal(t, input, out)
}

func TestRedactSensitive_SinglePassNoDuplication(t *testing.T) {
	// Verify that YAML-style lines (key: value) are redacted exactly once
	// and the separator format is preserved.
	out := redactSensitive("password: secret\n")
	assert.Equal(t, "password: [REDACTED]\n", out)
	assert.Equal(t, 1, strings.Count(out, "[REDACTED]"))
}

// --- detectContentType ---

func TestDetectContentType(t *testing.T) {
	cases := []struct {
		integration string
		path        string
		want        string
		note        string
	}{
		{"redis", "/etc/redis/redis.conf", "redis_conf", ""},
		{"redis", "/etc/redis/custom.conf", "redis_conf", "any .conf for redis"},
		// Only redis has a registered .conf parser; other integrations are unrecognised.
		{"nginx", "/etc/nginx/nginx.conf", contentTypeUnknown, "nginx parser not yet registered"},
		{"redis", "/etc/redis/config.yaml", "yaml", ""},
		{"mysql", "/etc/mysql/my.json", "json", ""},
		{"anything", "/path/to/file.yml", "yaml", ""},
		{"anything", "/path/to/file.txt", contentTypeUnknown, ""},
		// TOML and ini are not yet supported.
		{"anything", "/path/to/file.toml", contentTypeUnknown, "toml not yet registered"},
	}
	for _, tc := range cases {
		name := fmt.Sprintf("%s_%s", tc.integration, filepath.Base(tc.path))
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, detectContentType(tc.integration, tc.path))
		})
	}
}
