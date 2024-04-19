// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package collectorcontrib defines the interface for the collectorcontrib component
package collectorcontrib

import (
	"go.opentelemetry.io/collector/otelcol"
)

// Component is the interface for the collector-contrib
type Component interface {
	OTelComponentFactories() (otelcol.Factories, error)
}
