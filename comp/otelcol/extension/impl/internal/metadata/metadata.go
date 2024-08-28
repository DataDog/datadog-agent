// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metadata defines the metadata for the OpenTelemetry Extension component.
package metadata

import (
	"go.opentelemetry.io/collector/component"
)

var (
	// Type is the OpenTelemetry type for the extenstion
	Type = component.MustNewType("datadog")
)

const (
	// ExtensionStability is the OpenTelemetry current stability level for the extenstion
	ExtensionStability = component.StabilityLevelDevelopment
)
