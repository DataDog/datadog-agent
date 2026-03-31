// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package connection

import (
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/config"
)

const (
	// maxActive configures the maximum number of instances of the kretprobe-probed functions handled simultaneously.
	// This value should be enough for typical workloads (e.g. some amount of processes blocked on the `accept` syscall).
	maxActive = 512
)

// NewTracer returns a new Tracer
func NewTracer(cfg *config.Config, telemetryComp telemetry.Component) (Tracer, error) {
	if cfg.EnableEbpfless {
		return newEbpfLessTracer(cfg)
	}

	return newEbpfTracer(cfg, telemetryComp)
}
