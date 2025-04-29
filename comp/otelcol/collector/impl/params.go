// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package collectorimpl provides the implementation of the collector component for OTel Agent
package collectorimpl

// Params defines the parameters for the configsync component.
type Params struct {
	// IsBYOC indicates if the otel agent was built with BYOC
	IsBYOC string
}

// NewParams creates a new instance of Params
func NewParams(IsBYOC string) Params {
	params := Params{
		IsBYOC: IsBYOC,
	}
	return params
}

// NewDefaultParams returns the default params for configsync
func NewDefaultParams() Params {
	return Params{}
}
