// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package nettrace

import (
	"fmt"
)

type params struct {
	report    bool
	tracefile string
	// see `netsh trace show CaptureFilterHelp`
	// see https://docs.microsoft.com/en-us/windows-server/administration/windows-commands/netsh-trace
	captureFilter string
}

// ToArgs returns the parameters as a string to be used in a command
func (p *params) ToArgs() string {
	args := ""
	if p.tracefile != "" {
		args += fmt.Sprintf(" tracefile='%s'", p.tracefile)
	}
	if p.report {
		args += " report=yes"
	} else {
		args += " report=disabled"
	}
	if p.captureFilter != "" {
		args += fmt.Sprintf(" %s", p.captureFilter)
	}
	return args
}

// WithReport sets the report parameter
func WithReport(report bool) Option {
	return func(params *params) {
		params.report = report
	}
}

// WithTraceFile sets the tracefile parameter
func WithTraceFile(tracefile string) Option {
	return func(params *params) {
		params.tracefile = tracefile
	}
}

// WithCaptureFilter sets the capture filter parameter
func WithCaptureFilter(captureFilter string) Option {
	return func(params *params) {
		params.captureFilter = captureFilter
	}
}

func newParams(opts ...Option) *params {
	params := &params{}

	// apply input options
	for _, opt := range opts {
		opt(params)
	}

	return params
}
