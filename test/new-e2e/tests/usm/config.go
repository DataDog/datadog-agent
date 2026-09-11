// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package usm contains E2E tests for Universal Service Monitoring
package usm

import (
	_ "embed"
	"strings"
)

// systemProbeConfig defines the system-probe configuration for USM HTTP monitoring.
//
//go:embed config/usm.yaml
var systemProbeConfig string

// systemProbeConfigDirect is systemProbeConfig with direct send enabled
// (payloads sent directly from system-probe instead of process-agent).
var systemProbeConfigDirect = strings.Replace(systemProbeConfig, "direct_send: false", "direct_send: true", 1)
