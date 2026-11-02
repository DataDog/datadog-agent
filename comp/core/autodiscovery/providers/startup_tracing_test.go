// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package providers

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil/logging"
)

// Exercise the actual JSON transport and concurrent scanner after Fx Started,
// when the original Fx trace buffer has already been flushed.
func TestStartupScanTracePayload(t *testing.T) {
	t.Setenv("DD_FX_TRACING_ENABLED", "true")
	cfg := configmock.New(t)
	cfg.Set("autoconf_config_files_num_workers", 4, model.SourceAgentRuntime)
	dir := t.TempDir()
	contents := []byte("init_config: {}\ninstances:\n  - password: startup-test-private-value\n")
	if err := os.WriteFile(filepath.Join(dir, "startup.yaml"), contents, 0600); err != nil {
		t.Fatal(err)
	}
	readerMu.Lock()
	reader, doOnce = nil, sync.Once{}
	readerMu.Unlock()
	t.Cleanup(func() { ResetReader(nil) })
	payloads := make(chan []*logging.Span, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var traces [][]*logging.Span
		if err := json.NewDecoder(r.Body).Decode(&traces); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for _, trace := range traces {
			if len(trace) > 0 && trace[0].Name == "autodiscovery.startup" {
				payloads <- trace
			}
		}
	}))
	defer server.Close()
	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	var tracer *logging.FxTracingLogger
	app := fx.New(logging.DefaultFxLoggingOption(), fx.Invoke(func(logger fxevent.Logger) {
		tracer = logger.(*logging.FxTracingLogger)
		tracer.EnableSpansSending(port)
	}))
	if err := app.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Stop(context.Background()) })
	root := tracer.StartStartupTrace("autodiscovery")
	InitConfigFilesReaderWithContext(root.Context(t.Context()), []string{dir})
	root.Finish(nil)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	select {
	case spans := <-payloads:
		if len(spans) != 2 || spans[1].Resource != "config.scan" || spans[1].ParentID != spans[0].SpanID {
			t.Fatalf("unexpected scan trace: %+v", spans)
		}
		scan := spans[1]
		if scan.Metrics["config.files"] != 1 || scan.Metrics["config.bytes"] != float64(len(contents)) || scan.Metrics["config.loaded"] != 1 {
			t.Fatalf("incorrect scan counters: %v", scan.Metrics)
		}
		if scan.Metrics["config.worker_limit"] != 4 || scan.Meta["config.with_formats"] != "false" || scan.Metrics["config.format_time_ns"] != 0 {
			t.Fatalf("initial scan unexpectedly built formats: %+v", scan)
		}
		if scan.Metrics["config.read_time_ns"] <= 0 || scan.Metrics["config.parse_time_ns"] <= 0 {
			t.Fatalf("missing worker timings: %v", scan.Metrics)
		}
		encoded, err := json.Marshal(spans)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "startup-test-private-value") || strings.Contains(string(encoded), dir) {
			t.Fatal("diagnostic payload included configuration contents or paths")
		}
	case <-ctx.Done():
		t.Fatal("diagnostic trace was not delivered after Fx Started")
	}
}
