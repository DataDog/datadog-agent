// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package logscompression provides the component for logs compression
package logscompression

// team: agent-log-pipelines

import (
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

// Component is the component type.
// The logscompression component provides a factory that returns a requested Compressor
// used when setting up the endpoints.
// (This is different from the metrics compressor which returns the requested Compressor
// by reading the configuration at load time).
type Component interface {
	NewCompressor(kind string, level int) compression.Compressor
}
