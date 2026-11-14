// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestSampleShutdown(t *testing.T) {
	if os.Getenv("DD_DYNINST_SHUTDOWN_CHILD") == "1" {
		main()
		return
	}

	for _, tc := range []struct {
		name   string
		signal os.Signal
		span   string
	}{
		{"SIGTERM", syscall.SIGTERM, "demo-round"},
		{"SIGINT", os.Interrupt, "demo-round"},
		{"stdin", nil, "sample.other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var uploaded bool
			var mu sync.Mutex
			agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/info" {
					io.WriteString(w, `{"endpoints":["/v0.4/traces"]}`)
					return
				}
				body, err := io.ReadAll(r.Body)
				if err == nil && r.URL.Path == "/v0.4/traces" && bytes.Contains(body, []byte(tc.span)) {
					mu.Lock()
					uploaded = true
					mu.Unlock()
				}
				io.WriteString(w, `{}`)
			}))
			defer agent.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSampleShutdown$")
			loop := ""
			if tc.signal != nil {
				loop = "1"
			}
			cmd.Env = append(os.Environ(), "DD_DYNINST_SHUTDOWN_CHILD=1", "DD_SAMPLE_LOOP="+loop,
				"DD_TRACE_AGENT_URL="+agent.URL, "DD_INSTRUMENTATION_TELEMETRY_ENABLED=false",
				"DD_REMOTE_CONFIGURATION_ENABLED=false", "DD_TRACE_STARTUP_LOGS=false")
			cmd.Stdin = strings.NewReader("\n")
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			output := &shutdownOutput{}
			if tc.signal != nil {
				// Sample output proves the loop is running. Signal during that round,
				// before the tracer's periodic flush, without a timing-based sleep.
				output.signal = func() error { return cmd.Process.Signal(tc.signal) }
			}
			cmd.Stdout = output
			if err := cmd.Run(); err != nil {
				t.Fatalf("sample did not shut down cleanly: %v\n%s", err, &stderr)
			}
			mu.Lock()
			defer mu.Unlock()
			if !uploaded {
				t.Fatalf("sample exited without uploading %q\n%s", tc.span, &stderr)
			}
		})
	}
}

type shutdownOutput struct {
	once   sync.Once
	signal func() error
}

func (w *shutdownOutput) Write(p []byte) (int, error) {
	var err error
	if w.signal != nil {
		w.once.Do(func() { err = w.signal() })
	}
	return len(p), err
}
