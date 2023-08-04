// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kfilters

import (
	"path"
	"strings"

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

func validateBasenameFilter(value rules.FilterValue) bool {
	switch value.Type {
	case eval.ScalarValueType:
		return true
	case eval.GlobValueType:
		basename := path.Base(value.Value.(string))
		if !strings.Contains(basename, "*") {
			return true
		}
	}

	return false
}

func oneBasenameCapabilities(event string) Capabilities {
	return Capabilities{
		event + ".file.path": {
			PolicyFlags:     PolicyFlagBasename,
			FieldValueTypes: eval.ScalarValueType | eval.PatternValueType | eval.GlobValueType,
			ValidateFnc:     validateBasenameFilter,
		},
		event + ".file.name": {
			PolicyFlags:     PolicyFlagBasename,
			FieldValueTypes: eval.ScalarValueType,
		},
	}
}

func twoBasenameCapabilities(event string, field1, field2 string) Capabilities {
	return Capabilities{
		event + "." + field1 + ".path": {
			PolicyFlags:     PolicyFlagBasename,
			FieldValueTypes: eval.ScalarValueType | eval.GlobValueType,
			ValidateFnc:     validateBasenameFilter,
		},
		event + "." + field1 + ".name": {
			PolicyFlags:     PolicyFlagBasename,
			FieldValueTypes: eval.ScalarValueType,
		},
		event + "." + field2 + ".path": {
			PolicyFlags:     PolicyFlagBasename,
			FieldValueTypes: eval.ScalarValueType | eval.GlobValueType,
			ValidateFnc:     validateBasenameFilter,
		},
		event + "." + field2 + ".name": {
			PolicyFlags:     PolicyFlagBasename,
			FieldValueTypes: eval.ScalarValueType,
		},
	}
}

// GetCapababilities returns all the filtering capabilities
func GetCapababilities() map[eval.EventType]rules.FieldCapabilities {
	capabilities := make(map[eval.EventType]rules.FieldCapabilities)
	for eventType, eventCapabilities := range allCapabilities {
		capabilities[eventType] = eventCapabilities.GetFieldCapabilities()
	}
	return capabilities
}

func init() {
	allCapabilities["chmod"] = oneBasenameCapabilities("chmod")
	allCapabilities["chown"] = oneBasenameCapabilities("chown")
	allCapabilities["link"] = twoBasenameCapabilities("link", "file", "file.destination")
	allCapabilities["mkdir"] = oneBasenameCapabilities("mkdir")
	allCapabilities["open"] = openCapabilities
	allCapabilities["rename"] = twoBasenameCapabilities("rename", "file", "file.destination")
	allCapabilities["rmdir"] = oneBasenameCapabilities("rmdir")
	allCapabilities["unlink"] = oneBasenameCapabilities("unlink")
	allCapabilities["utimes"] = oneBasenameCapabilities("utimes")
	allCapabilities["mmap"] = mmapCapabilities
	allCapabilities["mprotect"] = mprotectCapabilities
	allCapabilities["splice"] = spliceCapabilities
}
