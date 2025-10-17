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

// Capababilities is a map of event types to field capabilities
type Capababilities map[eval.EventType]rules.FieldCapabilities

// allCapabilities hold all the supported filtering capabilities
var allCapabilities = make(Capababilities)

// GetCapababilities returns all the filtering capabilities
func GetCapababilities() Capababilities {
	return allCapabilities
}

// Clone returns a copy of the Capababilities
func (c *Capababilities) Clone() Capababilities {
	clone := make(Capababilities, len(*c))
	for k, v := range *c {
		clone[k] = v.Clone()
	}
	return clone
}
