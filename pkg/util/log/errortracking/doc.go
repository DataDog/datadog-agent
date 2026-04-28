// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package errortracking forwards Agent error logs to Datadog Cross-Org Agent
// Telemetry (COAT). It exposes an slog.Handler that captures records at
// level >= Error and submits them to an in-package Pipeline (Source channel
// -> Processors -> batched Sender). The Sender interface is the boundary
// between this package (foundational, dependency-light) and the existing
// COAT component at comp/core/agenttelemetry/, whose SendErrorLogs method
// implements Sender and is wired in via Fx.
//
// The package is disabled by default; the agent must be configured with
// errortracking.enabled = true to install the handler in the logger chain.
//
// The Processors slice on the Pipeline is the extension surface for future
// filters such as sampling, PII scrubbing, rate-limiting or fingerprinting.
// A Noop processor is provided as scaffolding so the slice is never empty.
package errortracking
