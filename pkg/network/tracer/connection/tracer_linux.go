// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package connection

import (
	"github.com/cilium/ebpf"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/network/config"
)

const (
	// maxActive configures the maximum number of instances of the kretprobe-probed functions handled simultaneously.
	// This value should be enough for typical workloads (e.g. some amount of processes blocked on the `accept` syscall).
	maxActive = 512
)

// Option configures the eBPF connection tracer
type Option func(*options)

type options struct {
	conntrackNATMap *ebpf.Map
}

// WithConntrackNATMap shares the eBPF conntracker's map of NAT'd connections with the tracer. Protocol
// classification then maps a socket's tuple to its packets' tuple from it, and the per-packet net_dev_queue
// probe isn't attached.
func WithConntrackNATMap(m *ebpf.Map) Option {
	return func(o *options) { o.conntrackNATMap = m }
}

// NewTracer returns a new Tracer
func NewTracer(cfg *config.Config, telemetryComp telemetry.Component, opts ...Option) (Tracer, error) {
	if cfg.EnableEbpfless {
		return newEbpfLessTracer(cfg)
	}

	var o options
	for _, opt := range opts {
		opt(&o)
	}
	return newEbpfTracer(cfg, telemetryComp, o)
}
