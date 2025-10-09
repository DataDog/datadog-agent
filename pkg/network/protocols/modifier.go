// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package protocols

import (
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
)

// ModifierProvider is an interface that protocols can implement to provide
// additional modifiers (like perf.EventHandler) that need to be attached
// to the ebpf-manager during initialization.
type ModifierProvider interface {
	// Modifiers returns the list of ddebpf.Modifier instances
	// the protocol wants attached to the ebpf-manager (usually a perf.EventHandler).
	Modifiers() []ddebpf.Modifier
}
