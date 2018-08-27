// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build !kubeapiserver

import "errors"

var (
	// ErrNotCompiled is returned if cluster check support is not compiled in.
	// User classes should handle that case as gracefully as possible.
	ErrNotCompiled = errors.New("intake support not compiled in")
)

type Intake struct{}

func NewIntake(_ interface{}) *Intake {
	return nil, ErrNotCompiled
}

type BufferedProcessor struct{}

func NewBufferedProcessor() (*BufferedProcessor, error)
	return nil, ErrNotCompiled
}
