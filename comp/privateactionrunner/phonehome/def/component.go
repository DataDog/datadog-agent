// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package phonehome defines the opt-in core-owned PAR startup gate.
package phonehome

// team: action-platform

// Component exposes startup-gate progress, not enrollment or action readiness.
type Component interface{ Status() *State }

// State describes the startup handoff, not enrollment or action readiness.
type State struct {
	State     string `json:"state"`
	Reason    string `json:"reason"`
	NextCheck string `json:"next_check,omitempty"`
}
