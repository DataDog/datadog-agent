// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package agentstackmonitor observes Datadog cluster-agent and
// cluster-check-runner pods co-scheduled on the node and reports
// sustained health problems (memory pressure, restarts, OOMKilled,
// CrashLoopBackOff) as healthplatform issues, while also exposing raw
// resource signals via pkg/telemetry for internal-org egress.
package agentstackmonitor

// team: container-platform

// Component has no external API.
type Component interface{}
