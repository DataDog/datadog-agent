// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build serverless

package telemetry

import (
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
)

// GetCompatComponent returns a component wrapping telemetry global variables
func GetCompatComponent() telemetry.Component {
	return noopsimpl.GetCompatComponent()
}
