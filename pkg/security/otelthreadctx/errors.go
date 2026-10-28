// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package otelthreadctx

import "errors"

// Sentinels the eBPF resolver's span context stats classify errors by.
var (
	// ErrNotApplicable marks a process that is instrumented, but not through
	// the reader that returned this error.
	ErrNotApplicable = errors.New("not applicable")
	// ErrUnsupported marks a shape this code recognizes but cannot handle
	ErrUnsupported = errors.New("unsupported")
	// ErrMalformed marks a process that violated the contract it published
	// under
	ErrMalformed = errors.New("malformed")
)
