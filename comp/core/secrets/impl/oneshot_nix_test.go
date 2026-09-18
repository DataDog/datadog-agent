// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows && test

package secretsimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/impl/noops"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/stretchr/testify/require"
)

func oneShotScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "backend")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0700))
	return path
}

func TestOneShotNativeAndMultipleBackendProtocol(t *testing.T) {
	for _, multi := range []bool{false, true} {
		t.Run(strconv.FormatBool(multi), func(t *testing.T) {
			requestFile := filepath.Join(t.TempDir(), "requests.json")
			script := oneShotScript(t, "cat >> \"$1\"\nprintf '\\n' >> \"$1\"\nprintf '%s' '{\"key\":{\"value\":\"resolved\"}}'\n")
			params := secrets.ConfigParams{Type: "json", Config: map[string]interface{}{"file": "native-file"}, Timeout: 10, MaxSize: 4096, Arguments: []string{requestFile}}
			input := "api_key: ENC[key]"
			want := map[string]string{"json": "native-file"}
			if multi {
				params.Type, params.Config = "", nil
				params.MultiBackends = map[string]secrets.SecretBackendConfig{
					"one": {Type: "json", Config: map[string]interface{}{"file": "one-file"}},
					"two": {Type: "yaml", Config: map[string]interface{}{"file": "two-file"}},
				}
				input = "api_key: ENC[one;key]\nproxy: ENC[two;key]"
				want = map[string]string{"json": "one-file", "yaml": "two-file"}
			}
			r := NewOneShotResolver(context.Background(), nooptelemetry.GetCompatComponent()).(*secretResolver)
			r.Configure(params)
			// Substitute only the installed connector executable; keep the real
			// native/grouped protocol, args and resolution machinery.
			r.backendCommand = script
			resolved, err := r.Resolve([]byte(input), "startup", "", "", false)
			require.NoError(t, err)
			require.NotContains(t, string(resolved), "ENC[")
			raw, err := os.ReadFile(requestFile)
			require.NoError(t, err)
			decoder := json.NewDecoder(bytes.NewReader(raw))
			got := map[string]string{}
			for {
				var request struct {
					Version string            `json:"version"`
					Type    string            `json:"type"`
					Config  map[string]string `json:"config"`
					Secrets []string          `json:"secrets"`
				}
				err := decoder.Decode(&request)
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
				require.Equal(t, secrets.PayloadVersion, request.Version)
				require.Equal(t, []string{"key"}, request.Secrets)
				got[request.Type] = request.Config["file"]
			}
			require.Equal(t, want, got)
		})
	}
}

func TestOneShotQuietProtocol(t *testing.T) {
	var mu sync.Mutex
	var messages []string
	log.SetLogObserver(func(_ log.LogLevel, msg string) { mu.Lock(); defer mu.Unlock(); messages = append(messages, msg) })
	t.Cleanup(func() { log.SetLogObserver(nil) })
	for _, response := range []string{`{"handle-sentinel":{"value":"value-sentinel"}}`, `{"handle-sentinel":{"error":"error-sentinel"}}`} {
		request := filepath.Join(t.TempDir(), "request.json")
		script := oneShotScript(t, "cat > \"$1\"\necho stderr-sentinel >&2\nprintf '%s' '"+response+"'\n")
		r := NewOneShotResolver(context.Background(), nooptelemetry.GetCompatComponent())
		r.Configure(secrets.ConfigParams{Command: script, Arguments: []string{request}, Timeout: 10, MaxSize: 4096})
		resolved, err := r.Resolve([]byte("api_key: ENC[handle-sentinel]"), "startup", "", "", false)
		if strings.Contains(response, "error") {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.Contains(t, string(resolved), "value-sentinel")
		}
		raw, err := os.ReadFile(request)
		require.NoError(t, err)
		var input struct {
			Secrets []string `json:"secrets"`
		}
		require.NoError(t, json.Unmarshal(raw, &input))
		require.Equal(t, []string{"handle-sentinel"}, input.Secrets)
	}
	mu.Lock()
	defer mu.Unlock()
	require.NotContains(t, strings.Join(messages, "\n"), "sentinel")
}

func TestOneShotCanceledBeforeCommand(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	script := oneShotScript(t, "touch \"$1\"\nprintf '%s' '{\"key\":{\"value\":\"dummy\"}}'\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := NewOneShotResolver(ctx, nooptelemetry.GetCompatComponent())
	r.Configure(secrets.ConfigParams{Command: script, Arguments: []string{marker}, Timeout: 1, MaxSize: 4096})
	_, err := r.Resolve([]byte("api_key: ENC[key]"), "startup", "", "", false)
	require.Error(t, err)
	require.NoFileExists(t, marker)
}

func TestOneShotBoundsInheritedPipes(t *testing.T) {
	pidfile := filepath.Join(t.TempDir(), "pid")
	script := oneShotScript(t, "sleep 5 &\necho $! > \"$1\"\nprintf '%s' '{\"key\":{\"value\":\"dummy\"}}'\n")
	t.Cleanup(func() {
		data, err := os.ReadFile(pidfile)
		if err != nil {
			return
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			return
		}
		process, err := os.FindProcess(pid)
		if err == nil {
			_ = process.Kill()
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r := NewOneShotResolver(ctx, nooptelemetry.GetCompatComponent())
	r.Configure(secrets.ConfigParams{Command: script, Arguments: []string{pidfile}, Timeout: 10, MaxSize: 4096})
	start := time.Now()
	_, err := r.Resolve([]byte("api_key: ENC[key]"), "startup", "", "", false)
	require.Error(t, err)
	require.Less(t, time.Since(start), 3*time.Second)
}
