// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package process

import "github.com/DataDog/datadog-agent/pkg/dyninst/ir"

// Config represents the current instrumentation configuration for a process.
// It embeds Info so callers can conveniently access process metadata without
// additional lookups.
type Config struct {
	Info

	RuntimeID string
	Probes    []ir.ProbeDefinition
	// InvalidProbes are the probes configured for the process whose
	// definitions could not be parsed. They cannot be instrumented, but are
	// reported so that users can see what is wrong with them.
	InvalidProbes     []InvalidProbe
	ShouldUploadSymDB bool
}

// InvalidProbe identifies a probe whose definition could not be parsed, and
// says why.
type InvalidProbe struct {
	ID      string
	Version int
	Issue   ir.Issue
}

var _ ir.ProbeIDer = InvalidProbe{}

// GetID implements ir.ProbeIDer.
func (p InvalidProbe) GetID() string { return p.ID }

// GetVersion implements ir.ProbeIDer.
func (p InvalidProbe) GetVersion() int { return p.Version }

// ProcessesUpdate aggregates lifecycle removals and refreshed configuration
// for processes discovered by dynamic instrumentation components.
type ProcessesUpdate struct {
	Removals []ID
	Updates  []Config
}
