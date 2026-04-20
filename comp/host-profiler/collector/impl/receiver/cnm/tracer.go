// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package cnm

import (
	"context"
	"fmt"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/network/tracer"
)

// initTracer creates the real eBPF-based network tracer.
// This is guarded by the linux_bpf build tag so unit tests (which use mocks)
// don't need eBPF support.
func (r *cnmReceiver) initTracer(_ context.Context) error {
	netCfg := r.cfg.toNetworkConfig()

	supported, err := tracer.IsTracerSupportedByOS(netCfg.ExcludedBPFLinuxVersions)
	if !supported {
		return fmt.Errorf("CNM receiver: kernel not supported: %w", err)
	}

	telemetryComp := telemetryimpl.GetCompatComponent()
	statsdClient := &ddgostatsd.NoOpClient{}

	tr, err := tracer.NewTracer(netCfg, telemetryComp, statsdClient)
	if err != nil {
		return fmt.Errorf("CNM receiver: failed to create network tracer: %w", err)
	}

	r.source = tr
	r.stopper = tr
	return nil
}
