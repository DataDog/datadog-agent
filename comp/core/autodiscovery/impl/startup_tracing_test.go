// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package autodiscoveryimpl

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/fx"

	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	fxinstrumentation "github.com/DataDog/datadog-agent/comp/core/fxinstrumentation/fx"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil/logging"
)

func TestAutodiscoveryStartupTraceLifecycle(t *testing.T) {
	t.Setenv("DD_FX_TRACING_ENABLED", "true")
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
	reqs := preparationTestRequires(t)
	reqs.Config.Set("apm_config.receiver_port", port, model.SourceAgentRuntime)
	reqs.Params.PreloadConfigsOnStart = true
	var ac autodiscovery.Component
	app := fx.New(logging.DefaultFxLoggingOption(), fxutil.FxLifecycleAdapter(),
		fxinstrumentation.Module(),
		fx.Provide(func() config.Component { return reqs.Config }),
		fx.Provide(func(lc compdef.Lifecycle, tracer *logging.FxTracingLogger) autodiscovery.Component {
			reqs.Lc, reqs.FxTracer = lc, tracer
			return NewComponent(reqs).Comp
		}), fx.Populate(&ac))
	if err := app.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Stop(context.Background()) })
	if err := ac.LoadAndRun(t.Context()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	select {
	case spans := <-payloads:
		resources := make(map[string]bool)
		for _, span := range spans {
			resources[span.Resource] = true
			if span.TraceID != spans[0].TraceID || span.Start+span.Duration > spans[0].Start+spans[0].Duration {
				t.Fatalf("phase outside diagnostic root: %+v", span)
			}
		}
		for _, name := range []string{"preparation", "config.initialize", "providers.discover", "listeners.initialize", "listener.init", "preparation.wait", "providers.load", "provider.load"} {
			if !resources[name] {
				t.Errorf("missing startup phase %q", name)
			}
		}
		if spans[0].Meta["fx.trace_id"] == "" || spans[0].Meta["agent.version"] == "" || spans[0].Error != 0 {
			t.Fatalf("missing correlation/build metadata or failed startup: %+v", spans[0])
		}
	case <-ctx.Done():
		t.Fatal("Autodiscovery startup trace was not delivered")
	}
}
