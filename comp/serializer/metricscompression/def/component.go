// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metricscompression provides the component for metrics compression
package metricscompression

// team: agent-metric-pipelines

import (
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

// Component is the component type.
type Component interface {
	compression.Compressor
}
