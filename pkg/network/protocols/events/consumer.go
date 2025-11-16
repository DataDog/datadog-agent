// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package events

import (
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
)

var (
	// Debug bla bla
	Debug = false
)

// Consumer is an interface for event consumers
type Consumer[V any] interface {
	Start()
	Sync()
	Stop()
}

// KernelAdaptiveConsumer wraps either DirectConsumer or BatchConsumer based on kernel version
// and provides both Consumer interface and Modifier interface in a single struct
type KernelAdaptiveConsumer[V any] struct {
	Consumer[V]                   // Embedded interface for Start/Sync/Stop
	modifiers   []ddebpf.Modifier // Modifiers for eBPF manager
}

// Modifiers implements the ModifierProvider interface
func (k *KernelAdaptiveConsumer[V]) Modifiers() []ddebpf.Modifier {
	return k.modifiers
}

// NewKernelAdaptiveConsumer creates a new KernelAdaptiveConsumer with the given consumer and modifiers
func NewKernelAdaptiveConsumer[V any](consumer Consumer[V], modifiers []ddebpf.Modifier) *KernelAdaptiveConsumer[V] {
	return &KernelAdaptiveConsumer[V]{
		Consumer:  consumer,
		modifiers: modifiers,
	}
}
