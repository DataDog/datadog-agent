// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package errortracking exposes the COAT (Cross-Org Agent Telemetry) error log
// sender component. The implementation lives in the impl/ subpackage and is
// wired in via fx/. Consumers (notably the logger setup at
// pkg/util/log/setup) inject this component as the errortracking.Sender of
// the in-package Pipeline at pkg/util/log/errortracking.
package errortracking

import (
	"context"
	"log/slog"
)

// team: agent-runtimes

// Component ships a batch of slog.Records to the COAT intake. Its method set
// matches the errortracking.Sender contract defined in
// pkg/util/log/errortracking, so a Component value satisfies that interface
// implicitly via Go structural typing — Worker 3's wiring passes the
// component to NewPipeline without an explicit conversion.
type Component interface {
	Send(ctx context.Context, batch []slog.Record) error
}
