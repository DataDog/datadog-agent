// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux_bpf

// Package tcpqueuelength is the system-probe side of the TCP Queue Length check
package tcpqueuelength

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/tcpqueuelength/model"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
)

// Tracer is not implemented on non-linux systems
type Tracer struct{}

// NewTracer is not implemented on non-linux systems
func NewTracer(*ebpf.Config) (*Tracer, error) {
	return nil, ebpf.ErrNotImplemented
}

// Close is not implemented on non-linux systems
func (t *Tracer) Close() {}

// Get is not implemented on non-linux systems
func (t *Tracer) Get() []model.TCPQueueLengthStats {
	return nil
}

// GetAndFlush is not implemented on non-linux systems
func (t *Tracer) GetAndFlush() []model.TCPQueueLengthStats {
	return nil
}
