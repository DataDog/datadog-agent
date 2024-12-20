// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package compression provides a compression implementation based on the configuration or available build tags.
package compression

import (
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

// team: agent-metrics-logs

// Component is the component type.
type Component interface {
	NewCompressor(kind string, level int) compression.Compressor
}
