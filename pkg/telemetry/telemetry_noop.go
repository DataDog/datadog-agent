// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build serverless

package telemetry

import (
	telemetrydef "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	implnoop "github.com/DataDog/datadog-agent/comp/core/telemetry/impl-noop"
)

// GetCompatComponent returns a component wrapping telemetry global variables
func GetCompatComponent() telemetrydef.Component {
	return implnoop.GetCompatComponent()
}
