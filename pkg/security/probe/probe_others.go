// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build !linux && !windows

package probe

// PlatformProbe represents the no-op platform probe on unsupported platforms
type PlatformProbe struct {
}

// Probe represents the runtime security probe
type Probe struct{}
