// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includessbury.
// Copyright 2016-present Datadog, Inc.

// Package fx provides a no-op fx module for the reporter component.
// Wire this in unit tests that build the observer but do not need reporting.
package fx

import (
	reporter "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the no-op reporter component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(newNoopReporter),
	)
}

type noopReporterRequires struct{}
type noopReporterProvides struct {
	Comp reporter.Component
}

func newNoopReporter(_ noopReporterRequires) noopReporterProvides {
	return noopReporterProvides{Comp: &noopReporter{}}
}

type noopReporter struct{}

func (r *noopReporter) Name() string                      { return "noop_reporter" }
func (r *noopReporter) Report(_ reporter.ReportOutput)    {}
