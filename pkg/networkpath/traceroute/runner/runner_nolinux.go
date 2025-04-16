// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !linux

package runner

import (
	telemetryComponent "github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network"
)

func createGatewayLookup(_ telemetryComponent.Component) (network.GatewayLookup, uint32, error) {
	return nil, 0, nil
}
