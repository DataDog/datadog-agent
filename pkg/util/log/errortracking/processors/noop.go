// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package processors contains optional Processors that can be added to an
// errortracking Pipeline to filter, transform, sample, scrub or otherwise
// preprocess records before they reach the Sender.
package processors

import (
	"log/slog"

	"github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
)

// Noop returns a Processor that returns its input unchanged. It is the
// default scaffolding for the Pipeline's Processors slice - an explicit
// "nothing to do here yet" placeholder that future filters can be slotted
// alongside, or in place of, without requiring a re-architecture of the
// errortracking package.
func Noop() errortracking.Processor {
	return noopProcessor{}
}

type noopProcessor struct{}

// Process returns r unchanged. The Pipeline guarantees a non-nil r; behavior
// on nil input is undefined.
func (noopProcessor) Process(r *slog.Record) *slog.Record {
	return r
}
