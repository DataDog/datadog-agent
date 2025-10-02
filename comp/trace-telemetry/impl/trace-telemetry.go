// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package tracetelemetryimpl implements the trace-telemetry component interface
package tracetelemetryimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	tracetelemetry "github.com/DataDog/datadog-agent/comp/trace-telemetry/def"
)

// Requires defines the dependencies for the trace-telemetry component
type Requires struct {
	Lifecycle compdef.Lifecycle

	Config    config.Component
	Client    ipc.HTTPClient
	Telemetry telemetry.Component
}

// Provides defines the output of the trace-telemetry component
type Provides struct {
	Comp tracetelemetry.Component
}

type tracetelemetryImpl struct {
	config config.Component
	client ipc.HTTPClient
	metric telemetry.Gauge

	// whether the trace-agent is running
	running atomic.Bool
	// whether the trace-agent is sending data
	sending atomic.Bool

	ctx    context.Context
	cancel context.CancelFunc
}

// NewComponent creates a new trace-telemetry component
func NewComponent(reqs Requires) (Provides, error) {
	metric := reqs.Telemetry.NewGauge("trace", "running", []string{"state"}, "Whether the trace-agent is running and sending data")

	t := &tracetelemetryImpl{
		config: reqs.Config,
		client: reqs.Client,
		metric: metric,
	}
	provides := Provides{
		Comp: t,
	}

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: func(_ context.Context) error {
			t.Start()
			return nil
		},
		OnStop: func(_ context.Context) error {
			t.Stop()
			return nil
		},
	})

	return provides, nil
}

type traceAgentExpvars struct {
	TraceWriterValues traceWriter `json:"trace_writer"`
	StatsWriterValues statsWriter `json:"stats_writer"`
}

type traceWriter struct {
	Bytes int64 `json:"bytes"`
}

type statsWriter struct {
	Bytes int64 `json:"bytes"`
}

func (t *tracetelemetryImpl) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	t.ctx = ctx
	t.cancel = cancel

	go func() {
		// trace-agent resets the expvars every minute
		ticker := time.NewTicker(time.Second * 50)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				t.updateState()

				var state string
				if t.running.Load() {
					if t.sending.Load() {
						state = "working"
					} else {
						state = "idle"
					}
				} else {
					state = "off"
				}
				t.metric.Set(1, state)
			case <-t.ctx.Done():
				return
			}
		}
	}()
}

func (t *tracetelemetryImpl) getTraceAgentExpvars() (traceAgentExpvars, error) {
	port := t.config.GetInt("apm_config.debug.port")

	url := fmt.Sprintf("https://localhost:%d/debug/vars", port)
	resp, err := t.client.Get(url, httphelpers.WithCloseConnection)
	if err != nil {
		return traceAgentExpvars{}, err
	}

	var values traceAgentExpvars
	if err := json.Unmarshal(resp, &values); err != nil {
		return traceAgentExpvars{}, err
	}
	return values, nil
}

func (t *tracetelemetryImpl) updateState() {
	if t.sending.Load() {
		// if we've established that trace-agent is sending data, don't update the state
		return
	}

	values, err := t.getTraceAgentExpvars()
	if err != nil {
		// keep previous information that we had about trace-agent
		return
	}

	t.running.Store(true)
	if values.TraceWriterValues.Bytes > 0 || values.StatsWriterValues.Bytes > 0 {
		t.sending.Store(true)
	}
}

func (t *tracetelemetryImpl) Stop() {
	t.cancel()
}
