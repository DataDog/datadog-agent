// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package errortracking

import "log/slog"

// Processor transforms or filters a single record on its way through the
// Pipeline. Returning nil drops the record. Processors are chained in the
// order configured on Options.Processors; the output of one is the input of
// the next.
//
// Implementations MUST be safe for concurrent use; the Pipeline calls
// Process from a single goroutine, but Processors may be shared across
// pipelines or referenced from other goroutines.
//
// Processors operate on a *slog.Record so they can mutate the record in
// place (e.g. drop attributes) without forcing a Clone allocation on every
// record. They may also return a new pointer (e.g. when stripping fields
// requires building a fresh record) - the Pipeline always uses the returned
// pointer for the next stage.
type Processor interface {
	Process(r *slog.Record) *slog.Record
}
