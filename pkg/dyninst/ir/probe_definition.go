// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ir

// ProbeDefinition abstracts the configuration of a probe.
type ProbeDefinition interface {
	// GetID returns the ID of the probe.
	GetID() string
	// GetVersion returns the version of the probe.
	GetVersion() int
	// GetTags returns the tags of the probe.
	GetTags() []string
	// GetKind returns the kind of the probe.
	GetKind() ProbeKind
	// GetWhere returns the where clause of the probe.
	GetWhere() Where
	// GetCaptureConfig returns the capture configuration of the probe.
	GetCaptureConfig() CaptureConfig
	// ThrottleConfig returns the throttle configuration of the probe.
	GetThrottleConfig() ThrottleConfig
}

// Where is a where clause of a probe.
type Where interface {
	Where() // marker method
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
