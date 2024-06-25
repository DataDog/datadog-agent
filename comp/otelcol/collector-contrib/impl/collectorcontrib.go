// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package collectorcontrib defines the implementation of the collectorcontrib component
package collectorcontrib

import (
	collectorcontrib "github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/def"
	"go.opentelemetry.io/collector/otelcol"
)

type collectorcontribImpl struct{}

// NewComponent returns a new collectorcontrib component
func NewComponent() collectorcontrib.Component {
	return &collectorcontribImpl{}
}

// OTelComponentFactories returns all of the otel collector components that the collector-contrib supports
func (c *collectorcontribImpl) OTelComponentFactories() (otelcol.Factories, error) {
	return components()
}
