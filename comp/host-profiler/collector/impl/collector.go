// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build hostprofiler

// Package collectorimpl implements the collector component interface
package collectorimpl

import (
	collector "github.com/DataDog/datadog-agent/comp/host-profiler/collector/def"
)

// Requires defines the dependencies for the collector component
type Requires struct{}

// Provides defines the output of the collector component.
type Provides struct {
	Comp collector.Component
}

type collectorImpl struct{}

// NewComponent creates a new collector component
func NewComponent(reqs Requires) (Provides, error) {
	provides := Provides{
		Comp: &collectorImpl{},
	}
	return provides, nil
}

func (c *collectorImpl) Run() error {
	return nil
}
