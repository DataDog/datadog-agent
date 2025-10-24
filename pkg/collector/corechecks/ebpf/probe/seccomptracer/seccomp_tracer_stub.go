// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux_bpf

// Package seccomptracer is the system-probe side of the Seccomp Tracer check
package seccomptracer

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/seccomptracer/model"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Tracer is not implemented on non-linux systems
type Tracer struct{}

// NewTracer is not implemented on this OS
func NewTracer(_ *ebpf.Config) (*Tracer, error) {
	log.Warn("seccomp tracer is not supported on this platform")
	return nil, nil
}

// Close is not implemented on this OS
func (t *Tracer) Close() {}

// GetAndFlush is not implemented on this OS
func (t *Tracer) GetAndFlush() model.SeccompStats {
	return model.SeccompStats{}
}
