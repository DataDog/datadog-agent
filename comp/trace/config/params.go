// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

// Params defines the parameters for the trace-agent config component.
type Params struct {
	// traceConfFilePath is the path at which to look for configuration
	traceConfFilePath string
}

// NewParams creates a new instance of Params
func NewParams(options ...func(*Params)) Params {
	params := Params{}
	for _, o := range options {
		o(&params)
	}
	return params
}

func WithTraceConfFilePath(confFilePath string) func(*Params) {
	return func(b *Params) {
		b.traceConfFilePath = confFilePath
	}
}
