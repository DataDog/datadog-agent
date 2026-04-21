// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build serverless

// Package telemetryimpl provides the serverless stub for GetCompatComponent.
package telemetryimpl

import (
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	noopsimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl/noops"
)

// GetCompatComponent returns a noop telemetry component for serverless builds.
// TODO (components): Remove this when all telemetry is migrated to the component
func GetCompatComponent() telemetry.Component {
	return noopsimpl.GetCompatComponent()
}
