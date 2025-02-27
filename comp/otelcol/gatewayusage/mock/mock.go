// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package mock implements mock for gatewayusage component interface
package mock

import gatewayusage "github.com/DataDog/datadog-agent/comp/otelcol/gatewayusage/def"

type gatewayUsageMock struct{}

func (m *gatewayUsageMock) OnHost(_ string) {}
func (m *gatewayUsageMock) Gauge() float64  { return 0 }

// NewMock returns a new instance of gatewayUsageMock
func NewMock() gatewayusage.Component {
	return &gatewayUsageMock{}
}
