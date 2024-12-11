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
var allCapabilities = make(map[eval.EventType]rules.FieldCapabilities)

// GetCapababilities returns all the filtering capabilities
func GetCapababilities() map[eval.EventType]rules.FieldCapabilities {
	return allCapabilities
}
