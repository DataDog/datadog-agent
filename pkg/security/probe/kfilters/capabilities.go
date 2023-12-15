// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package kfilters holds kfilters related files
package kfilters

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// allCapabilities hold all the supported filtering capabilities
var allCapabilities = make(map[eval.EventType]Capabilities)

// Capability represents the type of values we are able to filter kernel side
type Capability struct {
	PolicyFlags     PolicyFlag
	FieldValueTypes eval.FieldValueType
	ValidateFnc     func(value rules.FilterValue) bool
	FilterWeight    int
}

// Capabilities represents the filtering capabilities for a set of fields
type Capabilities map[eval.Field]Capability

// GetFlags returns the policy flags for the set of capabilities
func (caps Capabilities) GetFlags() PolicyFlag {
	var flags PolicyFlag
	for _, cap := range caps {
		flags |= cap.PolicyFlags
	}
	return flags
}

// GetFields returns the fields associated with a set of capabilities
func (caps Capabilities) GetFields() []eval.Field {
	var fields []eval.Field

	for field := range caps {
		fields = append(fields, field)
	}

	return fields
}

// GetFieldCapabilities returns the field capabilities for a set of capabilities
func (caps Capabilities) GetFieldCapabilities() rules.FieldCapabilities {
	var fcs rules.FieldCapabilities

	for field, cap := range caps {
		fcs = append(fcs, rules.FieldCapability{
			Field:        field,
			Types:        cap.FieldValueTypes,
			ValidateFnc:  cap.ValidateFnc,
			FilterWeight: cap.FilterWeight,
		})
	}

	return fcs
}

// GetCapababilities returns all the filtering capabilities
func GetCapababilities() map[eval.EventType]rules.FieldCapabilities {
	capabilities := make(map[eval.EventType]rules.FieldCapabilities)
	for eventType, eventCapabilities := range allCapabilities {
		capabilities[eventType] = eventCapabilities.GetFieldCapabilities()
	}
	return capabilities
}
