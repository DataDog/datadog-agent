// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

import (
	"math"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
)

// ProbeDefinition abstracts the configuration of a probe.
type ProbeDefinition interface {
	// GetID returns the ID of the probe.
	GetID() string
	// GetVersion returns the version of the probe.
	GetVersion() int
	// GetTags returns the tags of the probe.
	GetTags() []string
	// GetKind returns the kind of the probe.
	GetKind() ir.ProbeKind
	// GetWhere returns the where clause of the probe.
	GetWhere() Where
	// GetCaptureConfig returns the capture configuration of the probe.
	GetCaptureConfig() CaptureConfig
	// ThrottleConfig returns the throttle configuration of the probe.
	GetThrottleConfig() ThrottleConfig
}

// Where is a where clause of a probe.
type Where interface {
	where() // marker method
}

// FunctionWhere is a where clause of a probe that is a function.
type FunctionWhere interface {
	Where
	Location() (functionName string)
}

// CaptureConfig is the capture configuration of a probe.
type CaptureConfig interface {
	GetMaxReferenceDepth() uint32
	GetMaxFieldCount() uint32
	GetMaxCollectionSize() uint32
}

// ThrottleConfig is the throttle configuration of a probe.
type ThrottleConfig interface {
	GetThrottlePeriodMs() uint32
	GetThrottleBudget() int64
}

type captureConfig rcjson.Capture

var _ CaptureConfig = (*captureConfig)(nil)

func (c *captureConfig) GetMaxReferenceDepth() uint32 {
	if c == nil {
		return math.MaxUint32
	}
	if c.MaxReferenceDepth < 0 {
		return 0
	}
	return uint32(c.MaxReferenceDepth)
}

func (c *captureConfig) GetMaxFieldCount() uint32 {
	if c == nil {
		return math.MaxUint32
	}
	if c.MaxFieldCount < 0 {
		return 0
	}
	return uint32(c.MaxFieldCount)
}

func (c *captureConfig) GetMaxCollectionSize() uint32 {
	if c == nil {
		return 0
	}
	if c.MaxCollectionSize < 0 {
		return 0
	}
	return uint32(c.MaxCollectionSize)
}

type functionWhere rcjson.Where

var _ Where = (*functionWhere)(nil)

func (m *functionWhere) Location() string {
	return m.MethodName
}

func (m *functionWhere) where() {}
