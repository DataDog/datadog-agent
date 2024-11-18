// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

// Package tags holds tags related files
package tags

import (
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
)

// NewResolver returns a new tags resolver
func NewResolver(telemetry telemetry.Component, tagger Tagger) Resolver {
	return NewDefaultResolver(telemetry, tagger)
}
