// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
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

func oneBasenameCapabilities(event string) Capabilities {
	return Capabilities{
		event + ".filename": {
			PolicyFlags:     PolicyFlagBasename,
			FieldValueTypes: eval.ScalarValueType,
		},
		event + ".basename": {
			PolicyFlags:     PolicyFlagBasename,
			FieldValueTypes: eval.ScalarValueType,
		},
	}
}

func twoBasenameCapabilities(event string, field1, field2 string) Capabilities {
	return Capabilities{
		event + "." + field1 + ".filename": {
			PolicyFlags:     PolicyFlagBasename,
			FieldValueTypes: eval.ScalarValueType,
		},
		event + "." + field1 + ".basename": {
			PolicyFlags:     PolicyFlagBasename,
			FieldValueTypes: eval.ScalarValueType,
		},
		event + "." + field2 + ".filename": {
			PolicyFlags:     PolicyFlagBasename,
			FieldValueTypes: eval.ScalarValueType,
		},
		event + "." + field2 + ".basename": {
			PolicyFlags:     PolicyFlagBasename,
			FieldValueTypes: eval.ScalarValueType,
		},
	}
}

func init() {
	allCapabilities["chmod"] = oneBasenameCapabilities("chmod")
	allCapabilities["chown"] = oneBasenameCapabilities("chown")
	allCapabilities["link"] = twoBasenameCapabilities("link", "source", "target")
	allCapabilities["mkdir"] = oneBasenameCapabilities("mkdir")
	allCapabilities["open"] = openCapabilities
	allCapabilities["rename"] = twoBasenameCapabilities("rename", "old", "new")
	allCapabilities["rmdir"] = oneBasenameCapabilities("rmdir")
	allCapabilities["unlink"] = oneBasenameCapabilities("unlink")
	allCapabilities["utimes"] = oneBasenameCapabilities("utimes")
}
