// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Contains BSD-2-Clause code (c) 2015-present Andrea Barberio

// Package probes provides the interfaces for probes and probe responses
package probes

// Probe represents a sent probe. Every protocol-specific probe has to implement
// this interface
type Probe interface {
	Validate() error
}

// ProbeResponse represents a response to a sent probe. Every protocol-specific
// probe response has to implement this interface
type ProbeResponse interface {
	Validate() error
	Matches(Probe) bool
}
