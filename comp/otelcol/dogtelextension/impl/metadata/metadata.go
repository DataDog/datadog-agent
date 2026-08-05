// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metadata provides OpenTelemetry component metadata for dogtelextension
package metadata

import (
	"go.opentelemetry.io/collector/component"
)

var (
	// Type is the component type for this extension
	Type = component.MustNewType("dogtel")

	// ScopeName is the scope name for telemetry produced by this extension
	ScopeName = "github.com/DataDog/datadog-agent/comp/otelcol/dogtelextension"
)
