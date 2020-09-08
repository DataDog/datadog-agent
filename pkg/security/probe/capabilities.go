// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

var allCapabilities = make(map[eval.EventType]Capabilities)

// Capability represents the type of values we are able to filter kernel side
type Capability struct {
	PolicyFlags     PolicyFlag
	FieldValueTypes eval.FieldValueType
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
			Field: field,
			Types: cap.FieldValueTypes,
		})
	}

	return fcs
}

// ErrCapabilityNotFound is returned when there is not capabilities defined for an event type
type ErrCapabilityNotFound struct {
	EventType eval.EventType
}

func (e ErrCapabilityNotFound) Error() string {
	return fmt.Sprintf("capability not found for event type `%s`", e.EventType)
}

func init() {
	allCapabilities["open"] = openCapabilities
}
