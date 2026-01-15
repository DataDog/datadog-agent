// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fgmdef defines the FGM component interface
package fgmdef

// team: agent-metrics-logs

// Component provides fine-grained container metrics collection.
//
// The FGM (Fine-Grained Metrics) component subscribes to WorkloadMeta for
// container lifecycle events and samples cgroup/procfs metrics at configured
// intervals, forwarding them to the Observer component.
//
// This component is Linux-only and requires CGO to interface with the Rust
// observer library.
type Component interface {
	// Component lifecycle is managed by Fx
}
